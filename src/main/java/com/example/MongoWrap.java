package com.example;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

import com.mongodb.MongoCommandException;
import org.bson.BsonTimestamp;
import org.bson.Document;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.zeroturnaround.exec.ProcessExecutor;
import org.zeroturnaround.exec.ProcessResult;
import org.zeroturnaround.exec.StartedProcess;

import com.mongodb.client.MongoClient;
import com.mongodb.client.MongoClients;
import com.mongodb.client.result.DeleteResult;

public class MongoWrap {
    static Logger logger = LoggerFactory.getLogger(MongoWrap.class);

    static final String ADDR = System.getenv("MONGO_ADDR");
    static final String PORT = System.getenv("MONGO_PORT");
    static final String HOST = ADDR+":"+PORT;
    static final String DB_PATH = System.getenv("DB_PATH");
    static final String RSNAME = System.getenv("RS_NAME");
    static final String NODE_ID = System.getenv("NODE_ID");
    static final String NOMAD_ADDR = System.getenv("NOMAD_ADDR");


    public static void main(String[] args) throws Exception {
        logger.info(String.format("Host:%s Port:%s DbPath:%s", ADDR, PORT, DB_PATH));
        StartedProcess proc = new ProcessExecutor()
                .command("mongod.exe"
                        ,"--dbpath", DB_PATH
                        ,"--bind_ip", ADDR
                        ,"--port", PORT
                         ,"--replSet", RSNAME
                )
                .destroyOnExit()
                .redirectOutput(System.out)
                .redirectError(System.err)
                // .redirectInput(System.in)
                .start();
        MongoClient cli = MongoClients.create(String.format("mongodb://%s/", HOST));
        initiateReplicaSet(cli);
        for(;;){
            try{
                Thread.sleep(5000);
                Document res = cli.getDatabase("admin").runCommand(new Document("replSetGetStatus ",1));
                System.out.println(res);

            }catch (Exception e){
                e.printStackTrace();
            }finally {
            }
        }
    }

    private static void initiateReplicaSet(MongoClient cli) {
        logger.info("Checking replicaset master", HOST);

        for(;;){
            try{
                String output = new ProcessExecutor()
                        .command("nomad.exe","var","get", "-item", "mongo", "services")
                        .timeout(5,TimeUnit.SECONDS)
                        .readOutput(true)
                        .execute().outputUTF8();
                if(HOST.equals(output.split(";")[0])){
                    try{
                        Document rsStatus = cli.getDatabase("admin").runCommand(new Document("replSetGetStatus",1));
                        logger.info(rsStatus.toString());
                    }catch(MongoCommandException e){
                        if(e.getErrorCode() == 94) {
                            List<Document> members = new ArrayList<>();
                            String[] hosts = output.split(";");
                            for(int i=0;i<hosts.length;i++){
                                members.add(new Document("_id",i)
                                        .append("host",hosts[0])
                                    );
                            }
                            logger.info("replset not initialized. initializing now...");
                            Document result = cli.getDatabase("admin").runCommand(new Document("replSetInitiate",
                                    new Document("_id",RSNAME)
                                            .append("members", members)
                            ));
                            logger.info(result.toString());
                            return;
                        }
                        logger.info(e.toString());
                    }
                }
                logger.info("Mongo members:", output);

            }catch (Exception e){

            }
        }
    }

}
