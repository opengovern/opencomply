package model

import (
	"github.com/kaytu-io/kaytu-engine/pkg/onboard/api"
	"strings"
)

type CredentialType string

const (
	CredentialTypeAutoAzure             CredentialType = "auto-azure"
	CredentialTypeAutoAws               CredentialType = "auto-aws"
	CredentialTypeManualAwsOrganization CredentialType = "manual-aws-org"
	CredentialTypeManualAzureSpn        CredentialType = "manual-azure-spn"
)

func (c CredentialType) IsManual() bool {
	for _, t := range GetManualCredentialTypes() {
		if t == c {
			return true
		}
	}
	return false
}

func GetCredentialTypes() []CredentialType {
	return []CredentialType{
		CredentialTypeAutoAzure,
		CredentialTypeAutoAws,
		CredentialTypeManualAwsOrganization,
		CredentialTypeManualAzureSpn,
	}
}

func GetAutoGeneratedCredentialTypes() []CredentialType {
	return []CredentialType{
		CredentialTypeAutoAzure,
		CredentialTypeAutoAws,
	}
}

func GetManualCredentialTypes() []CredentialType {
	return []CredentialType{
		CredentialTypeManualAwsOrganization,
		CredentialTypeManualAzureSpn,
	}
}

func (c CredentialType) ToApi() api.CredentialType {
	return api.CredentialType(c)
}

func ParseCredentialType(s string) CredentialType {
	for _, t := range GetCredentialTypes() {
		if strings.ToLower(string(t)) == strings.ToLower(s) {
			return t
		}
	}
	return ""
}

func ParseCredentialTypes(s []string) []CredentialType {
	var ctypes []CredentialType
	for _, t := range s {
		ctypes = append(ctypes, ParseCredentialType(t))
	}
	return ctypes
}
