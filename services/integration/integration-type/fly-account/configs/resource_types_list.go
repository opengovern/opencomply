package configs

var TablesToResourceTypes = map[string]string{
	"fly_app":     "Fly/App",
	"fly_machine": "Fly/Machine",
	"fly_volume":  "Fly/Volume",
	"fly_secret":  "Fly/Secret",
}

var ResourceTypesList = []string{
	"Fly/App",
	"Fly/Machine",
	"Fly/Volume",
	"Fly/Secret",
}
