package envconfig

import (
	"log"
	"time"

	"github.com/caarlos0/env/v6"
	_ "github.com/joho/godotenv/autoload"
)

type config struct {
	PASETO_PRIVATE_KEY       string        `env:"PASETO_PRIVATE_KEY,required"`
	PASETO_EXPIRATION        time.Duration `env:"PASETO_EXPIRATION,required"`
	APP_PORT                 int           `env:"APP_PORT,required"`
	AUTH_EULA                string        `env:"AUTH_EULA,required"`
	APP_NAME                 string        `env:"APP_NAME,required"`
	GIN_MODE                 string        `env:"GIN_MODE,required"`
	DB_HOST                  string        `env:"DB_HOST,required"`
	DB_USERNAME              string        `env:"DB_USERNAME,required"`
	DB_PASSWORD              string        `env:"DB_PASSWORD,required"`
	DB_NAME                  string        `env:"DB_NAME,required"`
	DB_PORT                  int           `env:"DB_PORT,required"`
	ALLOWED_ORIGIN           []string      `env:"ALLOWED_ORIGIN,required" envSeparator:","`
	PASETO_SIGNED_BY         string        `env:"PASETO_SIGNED_BY,required"`
	APTOS_FUNCTION_ID        string        `env:"APTOS_FUNCTION_ID,required"`
	APTOS_REPORT_FUNCTION_ID string        `env:"APTOS_REPORT_FUNCTION_ID,required"`
	GAS_UNITS                int           `env:"GAS_UNITS,required"`
	GAS_PRICE                int           `env:"GAS_PRICE,required"`
	NETWORK                  string        `env:"NETWORK,required"`
	NFT_STORAGE_KEY          string        `env:"NFT_STORAGE_KEY,required"`
	VERSION                  string        `env:"VERSION,notEmpty"`
	VPN_DEPLOYER_API_US_EAST string        `env:"VPN_DEPLOYER_API_US_EAST,notEmpty"`
	VPN_DEPLOYER_API_SG      string        `env:"VPN_DEPLOYER_API_SG,notEmpty"`
	EREBRUS_API_US_EAST      string        `env:"EREBRUS_API_US_EAST,notEmpty"`
	EREBRUS_API_SG           string        `env:"EREBRUS_API_SG,notEmpty"`
	GOOGLE_AUDIENCE          string        `env:"GOOGLE_AUDIENCE,notEmpty"`
	OPENAI_API_KEY           string        `env:"OPENAI_API_KEY,notEmpty"`
	EREBRUS_US               string        `env:"EREBRUS_US,notEmpty"`
	EREBRUS_SG               string        `env:"EREBRUS_SG,notEmpty"`
	EREBRUS_CA               string        `env:"EREBRUS_CA,notEmpty"`
	EREBRUS_EU               string        `env:"EREBRUS_EU,notEmpty"`
	EREBRUS_JP               string        `env:"EREBRUS_JP,notEmpty"`
	SOTREUS_US               string        `env:"SOTREUS_US,notEmpty"`
	SOTREUS_SG               string        `env:"SOTREUS_SG,notEmpty"`
	STRIPE_WEBHOOK_SECRET    string        `env:"STRIPE_WEBHOOK_SECRET,notEmpty"`
	STRIPE_SECRET_KEY        string        `env:"STRIPE_SECRET_KEY,notEmpty"`
}

var EnvVars config = config{}

func InitEnvVars() {

	if err := env.Parse(&EnvVars); err != nil {
		log.Fatalf("failed to parse EnvVars: %s", err)
	}
}
