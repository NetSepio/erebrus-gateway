package dbconfig

import (
	"fmt"
	"os"

	"github.com/NetSepio/erebrus-gateway/models"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

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

func DbMigrations() error {
	db := GetDb()

	func() {
		// SQL query to create the extension
		sql := `CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`

		// Execute the query
		result := db.Exec(sql)
		if result.Error != nil {
			log.Fatal("failed to create extention : ", result.Error)
		}
	}()

	if err := db.AutoMigrate(
		&models.PerksToken{},
		&models.PerkNFT{},
		&models.Agent{},
		&models.SubscriptionToken{},
		&models.SubscriptionNFT{},
		&models.NodeLog{},
		&models.NodeActivity{},
		&models.Node{},
		&models.User{},
		&models.Erebrus{},
		&models.Subscription{},
		&models.FormData{},
		&models.FlowId{},
		&models.UserFeedback{},
		&models.WifiNode{},
		&models.NodeDwifi{},
		&models.WalrusStorage{},
		&models.CyreneAIAgent{},
		&models.NFTSubscriptionMintAddress{},
		&models.Agent{},
	); err != nil {
		log.Fatal("failed to automigration :", err)
	}
	if err := db.Exec("SELECT setval('subscriptions_id_seq', (SELECT MAX(id) FROM subscriptions));").Error; err != nil {
		log.Fatal("failed to set sequence value :", err)
	}

	if err := func() error {
		query := `SELECT setval('subscriptions_id_seq', (SELECT COALESCE(MAX(id), 1) FROM subscriptions), true);`
		return db.Exec(query).Error
	}(); err != nil {
		logrus.Println("failed to set sequence value: ", err)
		return err
	}

	return nil
}
