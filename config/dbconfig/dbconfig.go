package dbconfig

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/NetSepio/erebrus-gateway/models"

	"gorm.io/driver/postgres"
)

var db *gorm.DB

// Return singleton instance of db, initiates it before if it is not initiated already
func GetDb() *gorm.DB {
	if db != nil {
		return db
	}
	var (
		host     = os.Getenv("DB_HOST")
		username = os.Getenv("DB_USERNAME")
		password = os.Getenv("DB_PASSWORD")
		dbname   = os.Getenv("DB_NAME")
		port     = os.Getenv("DB_PORT")
	)

	dns := fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable port=%s",
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

	return db
}

func DbInit() error {
	db := GetDb()

	if err := db.AutoMigrate(&models.Erebrus{}, &models.Node{}, &models.Subscription{}, &models.FormData{}); err != nil {
		log.Fatal(err)
	}
	return nil
}
