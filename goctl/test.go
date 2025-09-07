package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func testJob() {
	deleted := false
	val := int64(0)
	var hosts []string
	for i := 1; i < 7; i++ {
		hosts = append(hosts, fmt.Sprintf("10.10.5%d.1:27017", i))
	}
	for {
		defer func() {
			log.Println("defer")
			time.Sleep(3 * time.Second)
		}()
		log.Println("Connecting")

		connectOpts := options.Client().SetHosts(hosts).SetAuth(options.Credential{
			Username: "admin",
			Password: "123",
		}).SetReplicaSet("rs0").SetServerSelectionTimeout(3 * time.Second)
		cli, err := mongo.Connect(connectOpts)
		if err != nil {
			continue
		}
		if cli.Ping(context.TODO(), nil) != nil {
			log.Println("disconnect")
			continue
		}
		if !deleted {
			err = cli.Database("mtest").Collection("testdoc").Drop(context.TODO())
			log.Println("Drop", err)
			deleted = true
		}
		err = cli.Database("mtest").CreateCollection(context.TODO(), "testdoc")
		log.Println("create", err)
		count, err := cli.Database("mtest").Collection("testdoc").CountDocuments(context.TODO(), bson.D{})
		log.Println("Count:::", count)
		if val != count {
			log.Println("Missing documents. expected:", val)
			//val = count
		}
		for {
			_, err := cli.Database("mtest").Collection("testdoc").InsertOne(context.TODO(), bson.D{{Key: "a", Value: val}})
			if err == nil {
				val += 1
				log.Println("Insert", val)
			} else {
				log.Println("Insert failed")
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}
