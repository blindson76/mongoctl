package com.example.repl;

import java.util.Arrays;
import java.util.Objects;
import java.util.concurrent.*;

public abstract class ReplicaController <C extends Replica.CandidateReportType,S extends Replica.ReplicaSetSpecType,H extends Replica.HealthStatusType>{
    public void ControllerTask(C report, S spec, H health, Store<C,S,H> store){
        CompletableFuture<Objects> future = new CompletableFuture<>();
        int testVal = 0;
        Object lock = new Object();
        store.WatchCandidateReports(data->{
            synchronized (lock){
                System.out.println("A:"+data);
                future.complete(null);
            }
        });
        store.WatchHealthStatus(data->{
            System.out.println("H:"+data);
        });
        ScheduledFuture<?> timer = Executors.newSingleThreadScheduledExecutor().schedule(()->{
            System.out.println("timer handler");
        },5, TimeUnit.SECONDS);
        timer.cancel(true);

        try {
            future.get();
        } catch (InterruptedException | ExecutionException e) {
            throw new RuntimeException(e);
        }
    }
    abstract C collect();
    abstract S generateConfig(H[] reports);
    abstract void memberTask();
}
