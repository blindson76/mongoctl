package com.example;

import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

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

public class MongoPrestart {
    static Logger logger = LoggerFactory.getLogger(MongoPrestart.class);

    static final String ADDR = System.getenv("MONGO_ADDR");
    static final String PORT = System.getenv("MONGO_PORT");
    static final String HOST = ADDR+":"+PORT;
    static final String CSB_IP = System.getenv("CSB_IP");
    static final String DB_PATH = System.getenv("DB_PATH");
    static final String RSNAME = System.getenv("RS_NAME");
    static final String NODE_ID = System.getenv("NODE_ID");
    static final String NOMAD_ADDR = System.getenv("NOMAD_ADDR");
    

    public static void main(String[] args) throws Exception {
        logger.info(String.format("Host:%s Port:%s DbPath:%s", "localhost", PORT, DB_PATH));
        StartedProcess proc = new ProcessExecutor()
            .command("mongod.exe"
                    ,"--dbpath", DB_PATH
                    ,"--bind_ip", "localhost"
                    ,"--port", PORT
                    // ,"--replSet", RSNAME
            )
            .destroyOnExit()
            .redirectOutput(System.out)
            .redirectError(System.err)
            // .redirectInput(System.in)
            .start();
        MongoClient cli = MongoClients.create(String.format("mongodb://localhost:%s/", PORT));
        BsonTimestamp ts = getOpLog(cli);
        logger.info(ts.toString());
        boolean deleted = deleteRSConfig(cli, RSNAME);
        if (deleted){
            logger.info("Replicaset config has been deleted succesfully");
        } else {
            logger.error("Replicaset config couldn't deleted");
            System.exit(-1);
        }
        
        try {
            cli.getDatabase("admin").runCommand(new Document("shutdown", 1));
        } catch (Exception e) {
            logger.warn("mongo socket exception");
        }
        Future<ProcessResult> future = proc.getFuture();
        try {            
            int mongoExists = future.get(1, TimeUnit.MINUTES).getExitValue();
            setMemberStatus(ts);
            System.exit(mongoExists);
        } catch (TimeoutException e) {
            logger.warn("mongodb hasn't exits in time");
            new ProcessExecutor()
            .command("taskkill"
                    ,"/f"
                    ,"/im"
                    ,"mongod.exe"
            ).timeout(5, TimeUnit.SECONDS).execute().getExitValue();
            System.exit(-2);
        }

    }
    
    private static boolean  deleteRSConfig(MongoClient cli, String rs) {
        Document replSet = cli.getDatabase("local").getCollection("system.replset").find(new Document("_id", rs)).first();
        if (replSet == null){
            return true;
        }
        DeleteResult result = cli.getDatabase("local").getCollection("system.replset").deleteOne(new Document("_id", rs));
        return result.getDeletedCount() == 1;

    }

    private static BsonTimestamp getOpLog(MongoClient cli) {
        Document tsDoc = cli.getDatabase("local").getCollection("oplog.rs")
            .find()
            .sort(new Document("$natural",-1))
            .limit(1)
            .first();
        if (tsDoc == null){
            return new BsonTimestamp(0, 0);
        }
        BsonTimestamp ts = tsDoc.get("ts", BsonTimestamp.class);
        return ts;
    }
    private static boolean setMemberStatus(BsonTimestamp ts){
        try{
            ProcessResult processResult = new ProcessExecutor()
            .command("nomad.exe"
                ,"var"
                ,"purge"
                ,String.format("mongo/%s/ts", NODE_ID)
            )
            .environment("NOMAD_ADDR", NOMAD_ADDR)
            .readOutput(true)
            .timeout(5, TimeUnit.SECONDS)
            .destroyOnExit()
            .execute();
            logger.info(processResult.outputUTF8());

            processResult = new ProcessExecutor()
            .command("nomad.exe"
                ,"var"
                ,"put"
                ,"-force"
                ,String.format("mongo/%s/ts", NODE_ID)
                ,String.format("second=%d", ts.getTime())
                ,String.format("inc=%d", ts.getInc())
                ,String.format("host=%s:%s", CSB_IP, PORT)
            )
            .environment("NOMAD_ADDR", NOMAD_ADDR)
            .timeout(5, TimeUnit.SECONDS)
            .readOutput(true)
            .destroyOnExit()
            .execute();
            logger.info(processResult.outputString());
            logger.info("nomad exit with: "+processResult.getExitValue());
            return processResult.getExitValue() == 0;
        }catch(Exception e){
            logger.error(e.toString());

        }
        return false;

    }
}
