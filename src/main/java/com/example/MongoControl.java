package com.example;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.*;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.util.*;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.stream.Collector;
import java.util.stream.Collectors;
import java.util.stream.Stream;

import static java.util.stream.Collectors.toList;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import com.google.gson.Gson;
import org.zeroturnaround.exec.ProcessExecutor;
import org.zeroturnaround.exec.ProcessResult;

public class MongoControl {
    static Logger logger = LoggerFactory.getLogger(MongoControl.class);

    static final String NOMAD_ADDR = System.getenv("NOMAD_ADDR");
    private static boolean configured = false;
    private static Map<String, Variable> allMembers = new HashMap<>();
    private static Map<String, Variable> replicaSet = new HashMap<>();


    public static void main(String[] args) {
        NomadWatch watch = new NomadWatch("vars", "prefix=mongo");
        for (; ; ) {
            try {
                VariablePath[] vars = watch.watch(VariablePath[].class);
                HttpClient client = HttpClient.newHttpClient();
                Gson gson = new Gson();
                List<CompletableFuture<Variable>> requests = Arrays.asList(vars).stream()
                        .filter(v -> v.Path.matches("mongo/.*?/ts"))
                        .map(var -> {
                            try {
                                return new URI(String.format("%s/v1/var/%s", NOMAD_ADDR, var.Path)); // TODO Auto-generated catch block
                            } catch (URISyntaxException ex) {
                                return null;
                            }
                        })
                        .map(HttpRequest::newBuilder)
                        .map(reqBuilder -> reqBuilder.build())
                        .map(req -> client.sendAsync(req, HttpResponse.BodyHandlers.ofString()).thenApply(res -> res.body()))
                        .map(str -> str.thenApply(json -> gson.fromJson(json, Variable.class)))
                        .collect(Collectors.toList());
                CompletableFuture.allOf(requests.toArray(CompletableFuture[]::new))
                        .whenComplete((o, throwable) -> {
                            List<Variable> nodes = requests.stream()
                                    .filter(r -> r.isDone())
                                    .map(r -> {
                                        try {
                                            return r.get();
                                        } catch (Exception e) {
                                            Variable v = new Variable();
                                            v.Items = new TimeStamp();
                                            return v;
                                        }
                                    })
                                    .sorted((a, b) -> {
                                        if (a.Items.second == b.Items.second) {
                                            return a.Items.inc - b.Items.inc;
                                        }
                                        return a.Items.second - b.Items.second;
                                    })
                                    .collect(toList());
                            logger.info(nodes.toString());
                            updateMembers(nodes);
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
        Executors.newSingleThreadScheduledExecutor().schedule(()->{
            configureCluster();
        }, delay, TimeUnit.SECONDS);



    }

    private static void configureCluster() {
        logger.info("Configuring mongo replicaset");
        List<Variable> nodes = allMembers.values().stream()
            .sorted((a, b) -> {
                if (a.Items.second == b.Items.second) {
                    return a.Items.inc - b.Items.inc;
                }
                return a.Items.second - b.Items.second;
            })
            .limit(3)
            .toList();
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
            allMembers.values().forEach(node->{
                String nodeName = node.Path.split("/")[1];
                String nodeId = nodeMap.get(nodeName);
                logger.info("unset node role for {}", nodeName);
                try{
                    ProcessResult pr = new ProcessExecutor()
                            .command("nomad.exe", "var", "purge", String.format("roles/%s/mongo",nodeName))
                            .timeout(5, TimeUnit.SECONDS)
                            .readOutput(true)
                            .execute();
                    if(pr.exitValue() == 0){
                        logger.info(String.format("%s marked as %s", nodeName,"mongo"));
                    }else{
                        logger.warn("unset result: {}", pr.outputUTF8());
                    }
                }catch(Exception e){
                    logger.error("node {} role unset failed {}",nodeName, e);
                }
            });
            AtomicInteger i=new AtomicInteger(0);
            replicaSet.values().forEach(node->{
                String nodeName = node.Path.split("/")[1];
                String nodeId = nodeMap.get(nodeName);
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
            });
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
        configured = true;
    }



    protected static class VariablePath {
        String Namespace;
        String Path;
        String Lock;
        int CreateIndex;
        long ModifyIndex;
        long ModifyTime;
        long CreateTime;
    }

    protected static class Variable extends VariablePath {
        TimeStamp Items;

    }

    protected static class TimeStamp {
        int inc;
        int second;
        String host;
    }

}
