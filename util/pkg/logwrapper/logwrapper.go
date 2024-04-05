package logwrapper

import (
	"os"

	"github.com/NetSepio/erebrus-gateway/config/envconfig"
	"github.com/sirupsen/logrus"
)

var Log *logrus.Entry

func Init() {
	appName := envconfig.EnvVars.APP_NAME
	hostname, err := os.Hostname()

	Log = logrus.New().WithFields(logrus.Fields{
		"hostname": hostname,
		"appname":  appName,
	})
	if err != nil {
		Log.Warnf("Error in getting hostname: %v", err)
	}
	Log.Logger.SetFormatter(&logrus.JSONFormatter{})
}
