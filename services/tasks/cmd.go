package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgtype"
	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/opengovern/og-util/pkg/jq"
	"github.com/opengovern/og-util/pkg/koanf"
	"github.com/opengovern/opencomply/services/tasks/config"
	"github.com/opengovern/opencomply/services/tasks/db/models"
	"github.com/opengovern/opencomply/services/tasks/scheduler"
	"github.com/opengovern/opencomply/services/tasks/worker"
	"gopkg.in/yaml.v3"
	"io/fs"
	"io/ioutil"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"

	"github.com/opengovern/og-util/pkg/httpserver"
	"github.com/opengovern/og-util/pkg/postgres"
	"github.com/opengovern/opencomply/services/tasks/db"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	TasksPath string = "/tasks"
)

func Command() *cobra.Command {
	return &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			return start(cmd.Context())
		},
	}
}

// start runs both HTTP and GRPC server.
// GRPC server has Check method to ensure user is
// authenticated and authorized to perform an action.
// HTTP server has multiple endpoints to view and update
// the user roles.
func start(ctx context.Context) error {
	cfg := koanf.Provide("tasks", config.Config{})

	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}

	logger = logger.Named("tasks")

	//m := email.NewSendGridClient(mailApiKey, mailSender, mailSenderName, logger)

	// setup postgres connection
	postgresCfg := postgres.Config{
		Host:    cfg.Postgres.Host,
		Port:    cfg.Postgres.Port,
		User:    cfg.Postgres.Username,
		Passwd:  cfg.Postgres.Password,
		DB:      cfg.Postgres.DB,
		SSLMode: cfg.Postgres.SSLMode,
	}
	orm, err := postgres.NewClient(&postgresCfg, logger)
	if err != nil {
		return fmt.Errorf("new postgres client: %w", err)
	}

	db := db.Database{Orm: orm}
	fmt.Println("Connected to the postgres database: ", cfg.Postgres.DB)

	err = db.Initialize()
	if err != nil {
		return fmt.Errorf("new postgres client: %w", err)
	}

	jq, err := jq.New(cfg.NATS.URL, logger)
	if err != nil {
		logger.Error("Failed to create job queue", zap.Error(err))
		return err
	}

	kubeClient, err := NewKubeClient()
	if err != nil {
		return err
	}

	err = setupTasks(ctx, cfg, db, kubeClient)
	if err != nil {
		return err
	}

	mainScheduler := scheduler.NewMainScheduler(logger, db, jq)
	err = mainScheduler.Start(ctx)
	if err != nil {
		return err
	}

	errors := make(chan error, 1)
	go func() {
		routes := httpRoutes{
			logger:        logger,
			db:            db,
			mainScheduler: mainScheduler,
		}
		errors <- fmt.Errorf("http server: %w", httpserver.RegisterAndStart(ctx, logger, cfg.Http.Address, &routes))
	}()

	return <-errors

}

func NewKubeClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := kedav1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	kubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}

func setupTasks(ctx context.Context, cfg config.Config, db db.Database, kubeClient client.Client) error {
	err := filepath.WalkDir(TasksPath, func(path string, d fs.DirEntry, err error) error {
		if !(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
			return nil
		}

		file, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		var task worker.Task
		err = yaml.Unmarshal(file, &task)
		if err != nil {
			return err
		}

		natsJsonData, err := json.Marshal(task.NatsConfig)
		if err != nil {
			return err
		}

		var natsJsonb pgtype.JSONB
		err = natsJsonb.Set(natsJsonData)
		if err != nil {
			return err
		}

		scaleJsonData, err := json.Marshal(task.NatsConfig)
		if err != nil {
			return err
		}

		var scaleJsonb pgtype.JSONB
		err = scaleJsonb.Set(scaleJsonData)
		if err != nil {
			return err
		}

		err = db.CreateTask(&models.Task{
			ID:          task.ID,
			Name:        task.Name,
			Description: task.Description,
			ImageUrl:    task.ImageURL,
			Interval:    task.Interval,
			NatsConfig:  natsJsonb,
			ScaleConfig: scaleJsonb,
		})
		if err != nil {
			return err
		}

		currentNamespace, ok := os.LookupEnv("CURRENT_NAMESPACE")
		if !ok {
			return fmt.Errorf("current namespace lookup failed")
		}
		err = worker.CreateWorker(ctx, cfg, kubeClient, &task, currentNamespace)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
