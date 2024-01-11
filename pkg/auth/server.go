package auth

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	envoytype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/gogo/googleapis/google/rpc"
	"github.com/golang-jwt/jwt"
	"github.com/kaytu-io/kaytu-engine/pkg/auth/api"
	"github.com/kaytu-io/kaytu-engine/pkg/auth/auth0"
	"github.com/kaytu-io/kaytu-engine/pkg/auth/db"
	"github.com/kaytu-io/kaytu-engine/pkg/httpserver"
	api2 "github.com/kaytu-io/kaytu-engine/pkg/workspace/api"
	"github.com/kaytu-io/kaytu-engine/pkg/workspace/client"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/rpc/status"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"time"
)

type User struct {
	UserID   string
	Metadata auth0.Metadata
}

type Server struct {
	host string

	kaytuPublicKey  *rsa.PublicKey
	verifier        *oidc.IDTokenVerifier
	verifierNative  *oidc.IDTokenVerifier
	logger          *zap.Logger
	workspaceClient client.WorkspaceServiceClient
	db              db.Database
	auth0Service    *auth0.Service

	updateLoginUserList []User
	updateLogin         chan User
}

func (s *Server) GetWorkspaceIDByName(workspaceName string) (string, error) {
	workspaceMap, err := s.db.GetWorkspaceMapByName(workspaceName)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", fmt.Errorf("workspace does not exists %s", workspaceName)
		} else {
			s.logger.Error("failed to get workspace map by name", zap.Error(err))
			return "", err
		}
	}
	return workspaceMap.ID, nil
}

func (s *Server) UpdateLastLoginLoop() {
	for {
		finished := false
		for !finished {
			select {
			case userId := <-s.updateLogin:
				alreadyExists := false
				for _, user := range s.updateLoginUserList {
					if user.UserID == userId.UserID {
						alreadyExists = true
					}
				}

				if !alreadyExists {
					s.updateLoginUserList = append(s.updateLoginUserList, userId)
				}
			default:
				finished = true
			}
		}

		for i := 0; i < len(s.updateLoginUserList); i++ {
			user := s.updateLoginUserList[i]
			s.logger.Error("updating metadata", zap.String("userId", user.UserID))
			err := s.auth0Service.PatchUserAppMetadata(user.UserID, auth0.Metadata{
				LastLogin: user.Metadata.LastLogin,
			})
			if err != nil {
				s.logger.Error("failed to update user metadata", zap.String("userId", user.UserID), zap.Error(err))
			}

			s.updateLoginUserList = append(s.updateLoginUserList[:i], s.updateLoginUserList[i+1:]...)
			i--
		}
		time.Sleep(time.Second)
	}
}

func (s *Server) UpdateLastLogin(claim *userClaim) {
	timeNow := time.Now().Format("2006-01-02 15:04:05 MST")
	doUpdate := false
	if claim.MemberSince == nil {
		claim.MemberSince = &timeNow
		doUpdate = true
	}
	if claim.UserLastLogin == nil {
		claim.UserLastLogin = &timeNow
		doUpdate = true
	} else {
		tim, _ := time.Parse("2006-01-02 15:04:05 MST", *claim.UserLastLogin)
		if time.Now().After(tim.Add(15 * time.Minute)) {
			claim.UserLastLogin = &timeNow
			doUpdate = true
		}
	}

	if doUpdate {
		s.updateLogin <- User{
			UserID: claim.ExternalUserID,
			Metadata: auth0.Metadata{
				WorkspaceAccess: claim.WorkspaceAccess,
				GlobalAccess:    claim.GlobalAccess,
				ColorBlindMode:  claim.ColorBlindMode,
				Theme:           claim.Theme,
				MemberSince:     claim.MemberSince,
				LastLogin:       claim.UserLastLogin,
			},
		}
	}
}

func (s *Server) Check(ctx context.Context, req *envoyauth.CheckRequest) (*envoyauth.CheckResponse, error) {
	unAuth := &envoyauth.CheckResponse{
		Status: &status.Status{
			Code: int32(rpc.UNAUTHENTICATED),
		},
		HttpResponse: &envoyauth.CheckResponse_DeniedResponse{
			DeniedResponse: &envoyauth.DeniedHttpResponse{
				Status: &envoytype.HttpStatus{Code: 401},
				Body:   http.StatusText(http.StatusUnauthorized),
			},
		},
	}

	httpRequest := req.GetAttributes().GetRequest().GetHttp()
	headers := httpRequest.GetHeaders()

	user, err := s.Verify(ctx, headers[strings.ToLower(echo.HeaderAuthorization)])
	if err != nil {
		s.logger.Warn("denied access due to unsuccessful token verification",
			zap.String("reqId", httpRequest.Id),
			zap.String("path", httpRequest.Path),
			zap.String("method", httpRequest.Method),
			zap.Error(err))
		return unAuth, nil
	}

	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	if user.Email == "" {
		s.logger.Warn("denied access due to failure to get email from token",
			zap.String("reqId", httpRequest.Id),
			zap.String("path", httpRequest.Path),
			zap.String("method", httpRequest.Method),
			zap.Error(err))
		return unAuth, nil
	}

	workspaceName := strings.TrimPrefix(httpRequest.Path, "/")
	if idx := strings.Index(workspaceName, "/"); idx > 0 {
		workspaceName = workspaceName[:idx]
	}

	if headerWorkspace, ok := headers["workspace-name"]; ok {
		workspaceName = headerWorkspace
	}

	rb, err := s.GetWorkspaceByName(workspaceName, user)
	if err != nil {
		s.logger.Warn("denied access due to failure in getting workspace",
			zap.String("reqId", httpRequest.Id),
			zap.String("path", httpRequest.Path),
			zap.String("method", httpRequest.Method),
			zap.String("workspace", workspaceName),
			zap.Error(err))
		return unAuth, nil
	}

	s.UpdateLastLogin(user)

	return &envoyauth.CheckResponse{
		Status: &status.Status{
			Code: int32(rpc.OK),
		},

		HttpResponse: &envoyauth.CheckResponse_OkResponse{
			OkResponse: &envoyauth.OkHttpResponse{
				Headers: []*envoycore.HeaderValueOption{
					{
						Header: &envoycore.HeaderValue{
							Key:   httpserver.XKaytuWorkspaceIDHeader,
							Value: rb.WorkspaceID,
						},
					},
					{
						Header: &envoycore.HeaderValue{
							Key:   httpserver.XKaytuWorkspaceNameHeader,
							Value: rb.WorkspaceName,
						},
					},
					{
						Header: &envoycore.HeaderValue{
							Key:   httpserver.XKaytuUserIDHeader,
							Value: rb.UserID,
						},
					},
					{
						Header: &envoycore.HeaderValue{
							Key:   httpserver.XKaytuUserRoleHeader,
							Value: string(rb.RoleName),
						},
					},
				},
			},
		},
	}, nil
}

type userClaim struct {
	WorkspaceAccess map[string]api.Role `json:"https://app.kaytu.io/workspaceAccess"`
	GlobalAccess    *api.Role           `json:"https://app.kaytu.io/globalAccess"`
	Email           string              `json:"https://app.kaytu.io/email"`
	MemberSince     *string             `json:"https://app.kaytu.io/memberSince"`
	UserLastLogin   *string             `json:"https://app.kaytu.io/userLastLogin"`
	ColorBlindMode  *bool               `json:"https://app.kaytu.io/colorBlindMode"`
	Theme           *api.Theme          `json:"https://app.kaytu.io/theme"`

	ExternalUserID string `json:"sub"`
}

func (u userClaim) Valid() error {
	return nil
}

func (s *Server) Verify(ctx context.Context, authToken string) (*userClaim, error) {
	if !strings.HasPrefix(authToken, "Bearer ") {
		return nil, errors.New("invalid authorization token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authToken, "Bearer "))
	if token == "" {
		return nil, errors.New("missing authorization token")
	}

	var u userClaim
	t, err := s.verifierNative.Verify(context.Background(), token)
	if err == nil {
		if err := t.Claims(&u); err != nil {
			return nil, err
		}

		return &u, nil
	}

	t, err = s.verifier.Verify(context.Background(), token)
	if err == nil {
		if err := t.Claims(&u); err != nil {
			return nil, err
		}

		return &u, nil
	}

	_, errk := jwt.ParseWithClaims(token, &u, func(token *jwt.Token) (interface{}, error) {
		return s.kaytuPublicKey, nil
	})
	if errk == nil {
		return &u, nil
	} else {
		fmt.Println("failed to auth with kaytu cred due to", errk)
	}
	return nil, err
}

func (s *Server) GetWorkspaceByName(workspaceName string, user *userClaim) (api.RoleBinding, error) {
	var rb api.RoleBinding
	var limits api2.WorkspaceLimitsUsage

	if user.ExternalUserID == api.GodUserID {
		return api.RoleBinding{}, errors.New("claiming to be god is banned")
	}

	rb = api.RoleBinding{
		UserID:        user.ExternalUserID,
		WorkspaceID:   "",
		WorkspaceName: "",
		RoleName:      api.EditorRole,
	}

	if workspaceName != "kaytu" {
		workspaceID, err := s.GetWorkspaceIDByName(workspaceName)
		if err != nil {
			return rb, err
		}

		rb.UserID = user.ExternalUserID
		rb.WorkspaceName = workspaceName
		rb.WorkspaceID = workspaceID

		if rl, ok := user.WorkspaceAccess[workspaceID]; ok {
			rb.RoleName = rl
		} else if user.GlobalAccess != nil {
			rb.RoleName = *user.GlobalAccess
		} else {
			return rb, fmt.Errorf("access denied: %s", limits.ID)
		}
	}

	return rb, nil
}

func newAuth0OidcVerifier(ctx context.Context, auth0Domain, clientId string) (*oidc.IDTokenVerifier, error) {
	provider, err := oidc.NewProvider(ctx, auth0Domain)
	if err != nil {
		return nil, err
	}

	return provider.Verifier(&oidc.Config{
		ClientID:          clientId,
		SkipClientIDCheck: true,
	}), nil
}
