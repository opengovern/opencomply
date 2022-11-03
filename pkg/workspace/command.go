package workspace

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var (
	PostgresHost              = os.Getenv("POSTGRES_HOST")
	PostgresPort              = os.Getenv("POSTGRES_PORT")
	PostgresDBName            = os.Getenv("POSTGRES_DB")
	PostgresUser              = os.Getenv("POSTGRES_USERNAME")
	PostgresPassword          = os.Getenv("POSTGRES_PASSWORD")
	ServerAddr                = os.Getenv("SERVER_ADDR")
	DomainSuffix              = os.Getenv("DOMAIN_SUFFIX")
	RedisAddress              = os.Getenv("REDIS_ADDRESS")
	AuthBaseURL               = os.Getenv("AUTH_BASE_URL")
	OnboardTemplate           = os.Getenv("ONBOARD_BASE_URL")
	InventoryTemplate         = os.Getenv("INVENTORY_BASE_URL")
	AutoSuspendDurationString = os.Getenv("AUTO_SUSPEND_DURATION_MINUTES")
	KeibiHelmChartLocation    = os.Getenv("KEIBI_HELM_CHART_LOCATION")
)

type Config struct {
	Host                   string
	Port                   string
	User                   string
	Password               string
	DBName                 string
	ServerAddr             string
	DomainSuffix           string
	AuthBaseUrl            string
	RedisAddress           string
	KeibiHelmChartLocation string
	AutoSuspendDuration    time.Duration
}

func NewConfig() *Config {
	d, _ := strconv.ParseInt(AutoSuspendDurationString, 10, 64)
	return &Config{
		Host:                   PostgresHost,
		Port:                   PostgresPort,
		User:                   PostgresUser,
		Password:               PostgresPassword,
		DBName:                 PostgresDBName,
		ServerAddr:             ServerAddr,
		DomainSuffix:           DomainSuffix,
		RedisAddress:           RedisAddress,
		AuthBaseUrl:            AuthBaseURL,
		KeibiHelmChartLocation: KeibiHelmChartLocation,
		AutoSuspendDuration:    time.Duration(d) * time.Minute,
	}
}

func Command() *cobra.Command {
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := NewConfig()

			s, err := NewServer(cfg)
			if err != nil {
				return fmt.Errorf("new server: %w", err)
			}
			return s.Start()
		},
	}
	return cmd
}
