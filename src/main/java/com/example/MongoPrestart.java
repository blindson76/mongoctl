package com.example;

import java.util.Collections;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

import org.bson.BsonDocument;
import org.bson.BsonInt32;
import org.bson.BsonString;
import org.bson.BsonTimestamp;
import org.bson.Document;
import org.zeroturnaround.exec.ProcessExecutor;
import org.zeroturnaround.exec.ProcessResult;
import org.zeroturnaround.exec.StartedProcess;

import com.mongodb.BasicDBObject;
import com.mongodb.client.FindIterable;
import com.mongodb.client.MongoClient;
import com.mongodb.client.MongoClients;
import com.mongodb.client.result.DeleteResult;

public class MongoPrestart {

    public static void main(String[] args) throws Exception {
        StartedProcess proc = new ProcessExecutor()
                .command("mongod.exe"
                        ,"--dbpath", System.getenv("DB_PATH")
                        ,"--bind_ip", System.getenv("CSB_IP")
                        ,"--port", System.getenv("MONGO_PORT")
                        ,"--replSet", "rs0"
                )
                .destroyOnExit()
                .redirectOutput(System.out)
                .redirectError(System.err)
                .redirectInput(System.in)
                .start();
        MongoClient cli = MongoClients.create(String.format("mongodb://%s:%s/", System.getenv("CSB_IP"), System.getenv("MONGO_PORT")));
        try {
            Document res = cli.getDatabase("admin").runCommand(new Document("replSetGetStatus",1));
            System.out.println(res);
        } catch (Exception e) {
            System.out.println(e.getMessage());
        }
        for (String db : cli.listDatabaseNames()) {
            System.out.println(db);
            for (String tb : cli.getDatabase(db).listCollectionNames()) {
                System.out.println("  " + tb);
            }
        }
        try {
            System.out.println("Replsets");
            for (Document d : cli.getDatabase("local").getCollection("system.replset").find()) {
                System.err.println(d.toString());
            }

            DeleteResult result = cli.getDatabase("local").getCollection("system.replset").deleteOne(new BsonDocument("_id", new BsonString("rs0")));
            System.out.println(result.getDeletedCount() + " entry deleted");
            Document results = cli.getDatabase("local").getCollection("oplog.rs").find().sort(new BasicDBObject("$natural", new BsonInt32(-1))).limit(1).first();
            if(results != null){
                BsonTimestamp st = results.get("ts", BsonTimestamp.class);
                System.err.println(st.getTime());
                System.err.println(st.getInc());
                System.out.println(results.get("ts"));
            }
            System.out.println(cli.getDatabase("admin").runCommand(new Document("shutdown", 1)));
            //proc.getProcess().destroy();
            Thread.sleep(3000);
            System.out.println("Sleep done");
        } catch (Exception e) {
            System.out.println(e.getMessage());
            //e.printStackTrace();
        }
        Future<ProcessResult> future = proc.getFuture();
        System.out.println("done"+future.get(10, TimeUnit.MINUTES).getExitValue());
    }
    public static void createUser(com.mongodb.MongoClient cli){
        boolean userExists =  cli.getDatabase("admin").runCommand(new Document("usersInfo", new Document("user", "adminUser").append("db", "admin")))
            .getList("users", Document.class)
            .iterator()
            .hasNext();
        if(!userExists){
            System.out.println("not found");

        
            Document userResult = cli.getDatabase("admin").runCommand(new Document("createUser", "adminUser")
                    .append("pwd", "123")
                    .append("roles",
                            Collections.singletonList(new Document("role", "root")
                                    .append("db", "admin"))));
            System.out.println(userResult);

        }
    }
}
