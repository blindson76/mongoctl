package com.example;

import com.google.gson.Gson;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.zeroturnaround.exec.ProcessExecutor;
import org.zeroturnaround.exec.ProcessResult;

import java.io.IOException;
import java.net.URI;
import java.net.URISyntaxException;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.Arrays;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.stream.Collectors;

public class MongoWatcher {
    static Logger logger = LoggerFactory.getLogger(MongoWatcher.class);

    static final String NOMAD_ADDR = System.getenv("NOMAD_ADDR");
    private static Map<String, Variable> allMembers = new HashMap<>();
    private static Map<String, Variable> replicaSet = new HashMap<>();
    private static ScheduledFuture<?> configurer;

    public static void main(String[] args) {
        NomadWatch watch = new NomadWatch("allocations");
        for (; ; ) {
            try {
                Allocation[] vars = watch.watch(Allocation[].class);
                HttpClient client = HttpClient.newHttpClient();
                Gson gson = new Gson();
                List<CompletableFuture<MongoStatusVar>> requests = Arrays.asList(vars).stream()
//                        .filter(v -> v.Path.matches("mongo/.*?/ts"))
                        .map(var -> {
                            try {
                                return new URI(String.format("%s/v1/var/%s", NOMAD_ADDR, var.JobID)); // TODO Auto-generated catch block
                            } catch (URISyntaxException ex) {
                                return null;
                            }
                        })
                        .map(HttpRequest::newBuilder)
                        .map(reqBuilder -> reqBuilder.build())
                        .map(req -> client.sendAsync(req, HttpResponse.BodyHandlers.ofString()).thenApply(res -> res.body()))
                        .map(str -> str.thenApply(json -> gson.fromJson(json, MongoStatusVar.class)))
                        .toList();
                logger.info("reqs {}", requests);

                CompletableFuture.allOf(requests.toArray(CompletableFuture[]::new))
                        .whenComplete((o, throwable) -> {
                            List<MongoStatusVar> nodes = requests.stream()
                                    .filter(CompletableFuture::isDone)
                                    .map(r -> {
                                        try {
                                            return r.get();
                                        } catch (Exception e) {
                                            return null;
                                        }
                                    })
                                    .sorted((a, b) -> {
                                        if (a.Items.opLogSec == b.Items.opLogSec) {
                                            return a.Items.opLogInc - b.Items.opLogInc;
                                        }
                                        return a.Items.opLogSec - b.Items.opLogSec;
                                    })
                                    .toList();
                            logger.info(nodes.toString());
//                            updateMembers(nodes);
//                            configureCluster(nodes);
                        });
            } catch (IOException | URISyntaxException ex) {
                ex.printStackTrace();
            }
        }

    }

    private static void updateMembers(List<Variable> nodes) {
        List<String> currentNodes = nodes.stream().map(node->node.Items.host).toList();
        List<String> removed = allMembers.keySet().stream()
                .filter(host->!currentNodes.contains(host))
                .toList();
//        removed.stream().forEach(host->{
//            logger.info("removing node {} from members list", host);
//            allMembers.remove(host);
//        });
        logger.info("updateMembers {}",String.join(",", currentNodes));
        nodes.forEach(node->{
            allMembers.put(node.Items.host, node);
        });
        long delay = 0;
        if(allMembers.size()>5){
            logger.info("all members settled. Configuring without waiting");
            delay = 0;
        }else if(allMembers.size()>2){
            delay = 30;
            logger.info("reach required nodes count. wait for {} to configure", delay);
        }

        if(allMembers.size()<3){
            logger.info("no reach to available nodes count");
            return;
        }
        if (configurer != null){
            configurer.cancel(false);
        }
        configurer = Executors.newSingleThreadScheduledExecutor().schedule(()->{
            configureCluster();
        }, delay, TimeUnit.SECONDS);



    }

    synchronized private static void configureCluster() {
        logger.info("Configuring mongo replicaset");
        List<Variable> nodes = allMembers.values().stream()
            .sorted((b, a) -> {
                if (a.Items.second == b.Items.second) {
                    if(a.Items.inc == b.Items.inc){
                        return a.Items.host.compareTo(b.Items.host);
                    }
                    return a.Items.inc - b.Items.inc;
                }
                return a.Items.second - b.Items.second;
            })
            .limit(3)
            .toList();
        List<String> removedNodes = replicaSet.keySet().stream()
            .filter(h->!nodes.stream().anyMatch(n->n.Items.host.equals(h)))
            .collect(Collectors.toList());

        List<String> addedNodes = nodes.stream()
            .filter(h->!replicaSet.containsKey(h.Items.host))
            .map(h->h.Items.host)
            .collect(Collectors.toList());

        nodes.forEach(node->{
            replicaSet.put(node.Items.host, node);
        });


        logger.info("Initiating configuration");
        try {
            ProcessResult processResult = new ProcessExecutor()
                    .command("nomad.exe", "node", "status")
                    .timeout(5, TimeUnit.SECONDS)
                    .readOutput(true)
                    .execute();
            String[] lines = processResult.outputUTF8().split("\n");
            Map<String, String> nodeMap = new HashMap<>();
            for (int j = 1; j < lines.length; j++) {
                String[] fields = lines[j].split("\s+");
                nodeMap.put(fields[3], fields[0]);
            }
            
            logger.info("removing active roles");
            removedNodes.stream().forEach(removed->{
                Variable node = allMembers.get(removed);
                String nodeName = node.Path.split("/")[1];
                logger.info("unset node role for {}", nodeName);
                try{
                    ProcessResult pr = new ProcessExecutor()
                            .command("nomad.exe", "var", "purge", String.format("roles/%s/mongo",nodeName))
                            .timeout(5, TimeUnit.SECONDS)
                            .readOutput(true)
                            .execute();
                    if(pr.exitValue() == 0){
                        logger.info(String.format("%s unmarked as %s", nodeName,"mongo"));
                    }else{
                        logger.warn("unset result: {}", pr.outputUTF8());
                    }
                }catch(Exception e){
                    logger.error("node {} role unset failed {}",nodeName, e);
                }
                replicaSet.remove(removed);
            });
            AtomicInteger i=new AtomicInteger(0);
            addedNodes.stream().forEach(added->{
                Variable node = allMembers.get(added);
                String nodeName = node.Path.split("/")[1];
                String mongoRole = i.getAndIncrement()==0 ? "primary" : "secondary";
                String members = String.join(";",replicaSet.values().stream().map(n->n.Items.host).toList());
                logger.info("set node role for {}", nodeName);
                try{
                    ProcessResult pr = new ProcessExecutor()
                            .command("nomad.exe", "var", "put", "-force", String.format("roles/%s/mongo",nodeName)
                                    ,String.format("role=%s", mongoRole)
                                    ,String.format("members=%s", members))
                            .timeout(5, TimeUnit.SECONDS)
                            .readOutput(true)
                            .execute();
                    if(pr.exitValue() == 0){
                        logger.info(String.format("%s marked as %s", nodeName,"mongo"));
                    }
                }catch(Exception e){
                    logger.error("node {} role set failed {}",nodeName, e);
                }
                replicaSet.put(added, node);
            });
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
    }



    protected static class VariablePath {
        String Namespace;
        String Path;
        String Lock;
        int CreateIndex;
        long ModifyIndex;
        long ModifyTime;
        long CreateTime;
        @Override
        public String toString(){
            return String.format("Path=%s ns=%s", Path, Namespace);
        }
    }

    protected static class Variable extends VariablePath {
        TimeStamp Items;

    }
    protected static class MongoStatusVar extends VariablePath {
        MongoPrestart.MongoMemberStatus Items;

    }

    protected static class TimeStamp {
        int inc;
        int second;
        String host;
    }
    protected static class Allocation {
        String Name;
        String JobID;
        String NodeName;
        String TaskGroup;
    }

}
