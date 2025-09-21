package com.example.repl;

public class TestClass {
    public static void main(String[] args) {
        ConsulStore.ConsulConfig cfg = ConsulStore.ConsulConfig.builder()
                .consulAddr("http://10.10.51.1:8500")
                .candidateReportPath("test/status")
                .candidateType(new MongoController.CandidateReport())
                .build();
        ConsulStore<MongoController.CandidateReport, MongoController.ReplicaSetSpec, MongoController.HealthStatus> store = new ConsulStore<>(cfg);
        MongoController.CandidateReport cr = new MongoController.CandidateReport();
        cr.id = "df";
        cr.name = "name";

//        store.PutCandidateReport(cr.id, cr);
        store.WatchCandidateReports(c->{
            c.forEach(d->{
                System.out.println(d.id);
            });
        });
    }
}
