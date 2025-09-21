package com.example.repl;

import com.example.repl.mongo.MongoManager;
import com.example.repl.mongo.MongoManager.MongoConfig;
import com.google.gson.Gson;
import lombok.Builder;

import java.nio.charset.StandardCharsets;

public class MongoController extends ReplicaController<MongoController.CandidateReport, MongoController.ReplicaSetSpec, MongoController.HealthStatus> {

    @Override
    CandidateReport collect() {
        MongoConfig cfg = MongoConfig.builder().build();
        return null;
    }

    @Override
    ReplicaSetSpec generateConfig(HealthStatus[] reports) {
        return null;
    }

    @Override
    void memberTask() {

    }

    public static class CandidateReport extends Replica.CandidateReportType<CandidateReport> {
        String id;
        String name;
        String status;

        @Override
        public String GetId() {
            return null;
        }
        protected CandidateReport(){
            super(CandidateReport.class);
        }

    }
    public static class ReplicaSetSpec extends Replica.ReplicaSetSpecType<ReplicaSetSpec> {

        protected ReplicaSetSpec() {
            super(ReplicaSetSpec.class);
        }
    }

    public static class HealthStatus extends Replica.HealthStatusType<HealthStatus> {

        protected HealthStatus() {
            super(HealthStatus.class);
        }

        @Override
        public String GetId() {
            return null;
        }
    }
}
