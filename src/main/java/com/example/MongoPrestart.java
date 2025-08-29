package com.example;

import java.util.Collections;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.stream.Collectors;

import org.bson.BsonTimestamp;
import org.bson.Document;
import org.bson.types.ObjectId;
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
//        proc.getFuture().get();
        MongoClient cli = MongoClients.create(String.format("mongodb://localhost:%s/", PORT));
        createUser(cli);
        MongoMemberStatus memberStatus = getMemberStatus(cli);
        if (memberStatus != null){
            boolean success = setMemberStatus(memberStatus);
            if (success){
                logger.info("mongo member status updated successfully");
            }
        }
        
        try {
            cli.getDatabase("admin").runCommand(new Document("shutdown", 1));
        } catch (Exception e) {
            logger.warn("mongo socket exception");
        }
        Future<ProcessResult> future = proc.getFuture();
        try {            
            int mongoExists = future.get(1, TimeUnit.MINUTES).getExitValue();
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
    private static void createUser(MongoClient cli) {
        try{
            logger.info("checking adminUser existence");
            Document user = cli.getDatabase("admin").runCommand(new Document("usersInfo",
                    new Document("user", "adminUser").append("db","admin")));
            boolean exists = user.containsKey("users") && user.getList("users", Document.class).iterator().hasNext();
            if (exists){
                logger.info("adminUser already exist");
            }else{
                logger.info("creating adminUser");
                Document newUser = new Document("createUser", "adminUser")
                        .append("pwd","123")
                        .append("roles", Collections.singletonList(
                                new Document("role", "root")
                                        .append("db","admin")
                        ));
                Document createResult = cli.getDatabase("admin").runCommand(newUser);
                logger.info("adminUser created successfully {}", createResult);
            }
        }catch(Exception e){
            logger.error("adminUser create error {}", e);

        }
    }
    private static MongoMemberStatus getMemberStatus(MongoClient cli) {
        MongoMemberStatus memberStatus = new MongoMemberStatus();
        memberStatus.nodeId = NODE_ID;
        try{

            Document rsConfig = cli.getDatabase("local").getCollection("system.replset").find(new Document("_id",RSNAME)).first();
            String members = rsConfig.getList("members", Document.class).stream()
                    .map(member->member.getString("host"))
                    .collect(Collectors.joining(","));
            String replSetId = rsConfig.get("settings", Document.class).get("replicaSetId", ObjectId.class).toString();
            int term = rsConfig.getInteger("term");
            BsonTimestamp oplogTs = getOpLog(cli);

            memberStatus.members = members;
            memberStatus.opLogSec = oplogTs.getTime();
            memberStatus.opLogInc = oplogTs.getInc();
            memberStatus.term = term;
            memberStatus.replSetId = replSetId;
            return memberStatus;
        }catch (Exception e){
            System.err.println(e);

        }
        return memberStatus;
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
    private static boolean setMemberStatus(MongoMemberStatus memberStatus){
        try{

            ProcessResult processResult = new ProcessExecutor()
                    .command("nomad.exe"
                            , "var"
                            , "put"
                            , "-force"
                            , String.format("status/mongo/%s", NODE_ID)
                            , String.format("nodeId=%s", memberStatus.nodeId)
                            , String.format("replSetId=%s", memberStatus.replSetId)
                            , String.format("members=%s", memberStatus.members)
                            , String.format("term=%d", memberStatus.term)
                            , String.format("oplogSec=%d", memberStatus.opLogSec)
                            , String.format("oplogInc=%d", memberStatus.opLogInc)
                    )
//                    .environment("NOMAD_ADDR", NOMAD_ADDR)
                    .readOutput(true)
                    .timeout(5, TimeUnit.SECONDS)
                    .destroyOnExit()
                    .execute();
            logger.info(processResult.outputUTF8());
            return processResult.getExitValue() == 0;
        }catch(Exception e){
            logger.error(e.toString());

        }
        return false;

    }
    public static class MongoMemberStatus {
        String nodeId;
        String replSetId;
        int term;
        int opLogSec;
        int opLogInc;
        String members;

        @Override
        public String toString(){
            return String.format("NodeId=%s replSetId=%s term=%s opLogSec=%d opLogInc=%d members=%s", nodeId, replSetId, term, opLogSec, opLogInc, members);
        }
    }
}
