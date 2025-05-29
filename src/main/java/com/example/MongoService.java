package com.example;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.URI;
import java.net.URLConnection;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

import com.google.gson.Gson;
import com.mongodb.MongoClientSettings;
import com.mongodb.MongoCredential;
import com.mongodb.MongoException;
import com.mongodb.ServerAddress;
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

public class MongoService {
    static Logger logger = LoggerFactory.getLogger(MongoService.class);

    static final String ADDR = System.getenv("MONGO_ADDR");
    static final String PORT = System.getenv("MONGO_PORT");
    static final String HOST = ADDR+":"+PORT;
    static final String CSB_IP = System.getenv("CSB_IP");
    static final String DB_PATH = System.getenv("DB_PATH");
    static final String RSNAME = System.getenv("RS_NAME");
    static final String NODE_ID = System.getenv("NODE_ID");
    static final String NOMAD_ADDR = System.getenv("NOMAD_ADDR");

    private static MongoRole lastRole;
    private static MongoClient cli;
    private static StartedProcess mongoProc;


    public static void main(String[] args) throws Exception {
        logger.info(String.format("Host:%s Port:%s DbPath:%s", "localhost", PORT, DB_PATH));
        prepareMongo();
        watchMongoRole();


    }

    private static void watchMongoRole() {
        NomadWatch watcher = new NomadWatch("vars", String.format("prefix=roles/%s/mongo",NODE_ID));
        Gson gson = new Gson();
        while (true){
            try{
                MongoRole mongoRole = null;
                MongoControl.VariablePath[] paths = watcher.watch(MongoControl.VariablePath[].class);
                for(MongoControl.VariablePath path : paths){
                    URLConnection req = new URI(String.format("%s/v1/var/%s",NOMAD_ADDR, path.Path)).toURL().openConnection();
                    BufferedReader br = new BufferedReader(new InputStreamReader(req.getInputStream()));
                    StringBuilder sb = new StringBuilder();
                    String line;
                    while ((line = br.readLine())!= null){
                        sb.append(line);
                    }
                    MongoRoleVar role = gson.fromJson(sb.toString(), MongoRoleVar.class);
                    logger.info(sb.toString());
                    mongoRole = role.Items;
                }
                if(lastRole == null ^ mongoRole == null){
                    updateRole(mongoRole);
                }
                else if (lastRole != null && mongoRole != null){
                    updateRole(mongoRole);
                }

            }catch (Exception e){

            }
        }
    }

    private static void updateRole(MongoRole mongoRole) {
        if(mongoRole == null && lastRole != null){
            //kill mongo
            logger.info("killing mongo process");
            if(mongoProc!= null){
                try{
                    cli.getDatabase("admin").runCommand(new Document("shutdown", 1));
//                    cli.close();
                }catch(Exception e){
                    try {
                        int exitCode = mongoProc.getFuture().get(10, TimeUnit.SECONDS).getExitValue();
                        logger.info("mongod exited with {}", exitCode);
                    } catch (Exception ex) {
                        logger.error("Mongo shutdown error {}", e.getMessage());
                        logger.info("send kill to mongod process");

                        mongoProc.getProcess().destroy();
                    }
                }
            }
        }
        else if(lastRole == null){
            //initiate mongo
            initiateMongo(mongoRole);
        } else {
            // change member state master<->slave
        }
        lastRole = mongoRole;

    }

    private static void initiateMongo(MongoRole mongoRole) {
        try{

            logger.info("Initiating mongod instance as {} with dbPath={} host={} port={} replSet={} keyFile={}", mongoRole.role, DB_PATH, ADDR, PORT, RSNAME,
                    Path.of(System.getenv("NOMAD_TASK_DIR"),"keyfile").toAbsolutePath().toString());

            mongoProc = new ProcessExecutor()
                    .command("mongod.exe"
                            ,"--dbpath", DB_PATH
                            ,"--bind_ip", ADDR
                            ,"--port", PORT
                            ,"--replSet", RSNAME
                            ,"--auth"
                            ,"--keyFile", Path.of(System.getenv("NOMAD_TASK_DIR"),"keyfile").toAbsolutePath().toString()
                    )
                    .destroyOnExit()
                    .redirectOutput(System.out)
                    .redirectError(System.err)
                    .start();
            cli = MongoClients.create(
                    MongoClientSettings.builder()
                            .credential(MongoCredential.createCredential("adminUser","admin","123".toCharArray()))
                            .applyToClusterSettings(builder->
                                    builder.hosts(Collections.singletonList(new ServerAddress(ADDR, Integer.parseInt(PORT))))
                            )
                            .build()
            );
            if(mongoRole.role.equals("primary")){
                logger.info("Checking replication");
                checkReplication(mongoRole);
            }
        }catch (Exception e){

        }
    }

    private static void checkReplication(MongoRole role) {
        try{
            Document rs = cli.getDatabase("admin").runCommand( new Document("replSetGetStatus", 1));
            logger.info("replciation already enabled");
            logger.info(rs.toString());
            //reconfigure if needed
        }catch (MongoException e){
            logger.info("replgetstatus error: {}", e.getMessage());
            logger.info("replciation not initialized yet. initializng with {}", role.members);
            String[] hosts = role.members.split(";");
            List<Document> members = new ArrayList<>();
            for(int i=0; i<hosts.length;i++){
                members.add(new Document("_id",i).append("host", hosts[i]));
            }
            try{
                Document rs = cli.getDatabase("admin").runCommand(new Document("replSetInitiate",
                        new Document("_id", RSNAME).append("members", members)));
                logger.info("replset initiated: {}", rs.toString());
            }catch (Exception e1){
                logger.error("rs initiate error: {}", e1.getMessage());
            }

        }
    }

    private static void prepareMongo() throws Exception {
        StartedProcess proc = new ProcessExecutor()
                .command("mongod.exe"
                        ,"--dbpath", DB_PATH
                        ,"--bind_ip", "localhost"
                        ,"--port", PORT
                )
                .destroyOnExit()
//                .redirectOutput(System.out)
//                .redirectError(System.err)
                // .redirectInput(System.in)
                .start();
        MongoClient cli = MongoClients.create(String.format("mongodb://localhost:%s/", PORT));
        createUser(cli);
        BsonTimestamp ts = getOpLog(cli);
        boolean deleted = deleteRSConfig(cli, RSNAME);
        if (deleted){
            logger.info("Replicaset config has been deleted succesfully");
        } else {
            logger.error("Replicaset config couldn't deleted");
            throw new Exception("Replicaset config couldn't deleted");
        }

        try {
            cli.getDatabase("admin").runCommand(new Document("shutdown", 1));
        } catch (Exception e) {
            logger.warn("mongo socket exception while shutdown");
        }
        Future<ProcessResult> future = proc.getFuture();
        try {
            int mongoExists = future.get(1, TimeUnit.MINUTES).getExitValue();
            logger.info("mongod exit with: "+ mongoExists);
            setMemberStatus(ts);
        } catch (TimeoutException e) {
            logger.warn("mongodb hasn't exits in time");
            new ProcessExecutor()
                    .command("taskkill"
                            ,"/f"
                            ,"/im"
                            ,"mongod.exe"
                    ).timeout(5, TimeUnit.SECONDS).execute().getExitValue();

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
            if(processResult.getExitValue()==0){
                logger.info("mongo oplog timestamps updated successfully");
            }else{
                logger.warn("mongo oplog timestamps updated failed");
                logger.warn(processResult.outputString());
            }
            return processResult.getExitValue() == 0;
        }catch(Exception e){
            logger.error(e.toString());

        }
        return false;

    }
    private static class MongoRole {
        String role;
        String members;
    }
    private static class MongoRoleVar extends MongoControl.VariablePath{
        MongoRole Items;
    }
}
