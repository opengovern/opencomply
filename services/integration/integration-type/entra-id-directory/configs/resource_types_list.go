package configs

var TablesToResourceTypes = map[string]string{
	"entraid_group":                        "Microsoft.Entra/groups",
	"entraid_group_membership":             "Microsoft.Entra/groupMemberships",
	"entraid_device":                       "Microsoft.Entra/devices",
	"entraid_application":                  "Microsoft.Entra/applications",
	"entraid_app_registration":             "Microsoft.Entra/appRegistrations",
	"entraid_enterprise_application":       "Microsoft.Entra/enterpriseApplication",
	"entraid_managed_identity":             "Microsoft.Entra/managedIdentity",
	"entraid_microsoft_application":        "Microsoft.Entra/microsoftApplication",
	"entraid_domain":                       "Microsoft.Entra/domains",
	"entraid_tenant":                       "Microsoft.Entra/tenant",
	"entraid_identity_provider":            "Microsoft.Entra/identityproviders",
	"entraid_security_defaults_policy":     "Microsoft.Entra/securitydefaultspolicy",
	"entraid_authorization_policy":         "Microsoft.Entra/authorizationpolicy",
	"entraid_conditional_access_policy":    "Microsoft.Entra/conditionalaccesspolicy",
	"entraid_admin_consent_request_policy": "Microsoft.Entra/adminconsentrequestpolicy",
	"entraid_user_registration_details":    "Microsoft.Entra/userregistrationdetails",
	"entraid_service_principal":            "Microsoft.Entra/serviceprincipals",
	"entraid_user":                         "Microsoft.Entra/users",
	"entraid_directory_role":               "Microsoft.Entra/directoryroles",
	"entraid_directory_setting":            "Microsoft.Entra/directorysettings",
}

var ResourceTypesList = []string{
	"Microsoft.Entra/groups",
	"Microsoft.Entra/groupMemberships",
	"Microsoft.Entra/devices",
	"Microsoft.Entra/applications",
	"Microsoft.Entra/appRegistrations",
	"Microsoft.Entra/enterpriseApplication",
	"Microsoft.Entra/managedIdentity",
	"Microsoft.Entra/microsoftApplication",
	"Microsoft.Entra/domains",
	"Microsoft.Entra/tenant",
	"Microsoft.Entra/identityproviders",
	"Microsoft.Entra/securitydefaultspolicy",
	"Microsoft.Entra/authorizationpolicy",
	"Microsoft.Entra/conditionalaccesspolicy",
	"Microsoft.Entra/adminconsentrequestpolicy",
	"Microsoft.Entra/userregistrationdetails",
	"Microsoft.Entra/serviceprincipals",
	"Microsoft.Entra/users",
	"Microsoft.Entra/directoryroles",
	"Microsoft.Entra/directorysettings",
}
