package configs

var TablesToResourceTypes = map[string]string{
	"tailscale_device":          "TailScale/Device",
	"tailscale_user":            "TailScale/User",
	"tailscale_contact":         "TailScale/Contact",
	"tailscale_device_invite":   "TailScale/Device/Invite",
	"tailscale_device_posture":  "TailScale/Device/Posture",
	"tailscale_user_invite":     "TailScale/User/Invite",
	"tailscale_key":             "TailScale/Key",
	"tailscale_policy":          "TailScale/Policy",
	"tailscale_tailnet_setting": "TailScale/TailnetSetting",
	"tailscale_webhook":         "TailScale/Webhook",
	"tailscale_dns":             "TailScale/DNS",
}

var ResourceTypesList = []string{
	"TailScale/Device",
	"TailScale/User",
	"TailScale/Contact",
	"TailScale/Device/Invite",
	"TailScale/Device/Posture",
	"TailScale/User/Invite",
	"TailScale/Key",
	"TailScale/Policy",
	"TailScale/TailnetSetting",
	"TailScale/Webhook",
	"TailScale/DNS",
}
