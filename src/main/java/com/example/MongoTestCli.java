package com.example;

import com.mongodb.MongoClientSettings;
import com.mongodb.MongoCredential;
import com.mongodb.ServerAddress;
import com.mongodb.client.MongoClient;
import com.mongodb.client.MongoClients;
import org.bson.Document;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Collections;
import java.util.concurrent.TimeUnit;

public class MongoTestCli {
    static Logger logger = LoggerFactory.getLogger(MongoTestCli.class);
    public static void main(String[] args) throws Exception {
        MongoClient cli = MongoClients.create(
                MongoClientSettings.builder()
                        .credential(MongoCredential.createCredential("adminUser","admin","123".toCharArray()))
                        .applyToClusterSettings(builder->
                                builder.hosts(Collections.singletonList(new ServerAddress("10.10.11.1", 27015)))
                                        .serverSelectionTimeout(3, TimeUnit.SECONDS)
                        )
                        .build()
        );
        Document tsDoc = cli.getDatabase("admin").runCommand(new Document("replSetGetStatus",1));
        logger.info("Oplog result: {}", ((Document)tsDoc.get("optimes")).get("lastCommittedOpTime"));

    }
}
