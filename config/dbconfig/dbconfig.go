package dbconfig

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/NetSepio/erebrus-gateway/config/envconfig"

	"gorm.io/driver/postgres"
)

var db *gorm.DB

// Return singleton instance of db, initiates it before if it is not initiated already
func GetDb() *gorm.DB {
	if db != nil {
		return db
	}
	var (
		host     = envconfig.EnvVars.DB_HOST
		username = envconfig.EnvVars.DB_USERNAME
		password = envconfig.EnvVars.DB_PASSWORD
		dbname   = envconfig.EnvVars.DB_NAME
		port     = envconfig.EnvVars.DB_PORT
	)

	dns := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable port=%d",
		host, username, password, dbname, port)

	var err error
	db, err = gorm.Open(postgres.New(postgres.Config{
		DSN: dns,
	}))
	if err != nil {
		log.Fatal("failed to connect database", err)
	}

	sqlDb, err := db.DB()
	if err != nil {
		log.Fatal("failed to ping database", err)
	}
	if err = sqlDb.Ping(); err != nil {
		log.Fatal("failed to ping database", err)
	}

	// if err := db.AutoMigrate(&models.User{}, &models.Role{}, &models.UserFeedback{}, &models.FlowId{}, &models.Review{}, &models.WaitList{}, &models.Domain{}, &models.DomainAdmin{}, &models.Sotreus{}, &models.Erebrus{}); err != nil {
	// 	log.Fatal(err)
	// }

	// db.Exec(`create table if not exists user_roles (
	// 		wallet_address text,
	// 		role_id text,
	// 		unique (wallet_address,role_id)
	// 		)`)

	// //Create flow id
	// db.Exec(`
	// DO $$ BEGIN
	// 	CREATE TYPE flow_id_type AS ENUM (
	// 		'AUTH',
	// 		'ROLE');
	// EXCEPTION
	// 	WHEN duplicate_object THEN null;
	// END $$;`)

	return db.Debug()
}
