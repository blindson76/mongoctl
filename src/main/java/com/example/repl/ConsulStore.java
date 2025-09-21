package com.example.repl;

import com.ecwid.consul.v1.ConsulClient;
import com.ecwid.consul.v1.QueryParams;
import com.ecwid.consul.v1.Response;
import com.ecwid.consul.v1.kv.model.GetValue;
import lombok.Builder;

import java.nio.file.Paths;
import java.util.List;

public class ConsulStore<C extends Replica.CandidateReportType<C>,S extends Replica.ReplicaSetSpecType<S>,H extends Replica.HealthStatusType<H>> implements Store<C,S,H>{
    ConsulConfig<C,S,H> cfg;
    ConsulClient cli;
    public ConsulStore(ConsulConfig<C,S,H> cfg){
        this.cfg = cfg;
        this.cli = new ConsulClient(cfg.consulAddr);

    }
    @Override
    public void PutCandidateReport(String id, C val) {
        String path = Paths.get(cfg.candidateReportPath,id).normalize().toString().replace("\\", "/");
        cli.setKVValue(path, val.JSONBytes());

    }

    @Override
    public void WatchCandidateReports(Listener<List<C>> listener) {
        String key = Paths.get(cfg.candidateReportPath).normalize().toString().replace("\\", "/");
        long lastIndex = 0;
        QueryParams.Builder paramBuilder = QueryParams.Builder.builder().setIndex(lastIndex);
        for(;;){
            try{
                Response<List<GetValue>> val = cli.getKVValues(key, paramBuilder.build());
                if (val != null){

                    paramBuilder.setIndex(val.getConsulIndex());
                    List<C> result = val.getValue().stream().map(v-> cfg.candidateType.Parse(v.getDecodedValue()))
                            .toList();
                    listener.AcceptData(result);

                }
            }catch (Exception e){
                System.out.println("error");
                try {
                    Thread.sleep(2000);
                } catch (InterruptedException ex) {
                    throw new RuntimeException(ex);
                }
            }

        }

    }

    @Override
    public void UpdateHealthStatus(String id, H status) {

    }

    @Override
    public void WatchHealthStatus(Listener<List<H>> listener) {

    }

    @Override
    public void UpdateReplSetConfig(S spec) {

    }

    @Override
    public void WatchReplSetConfig(Listener<List<S>> listener) {

    }

    @Builder
    public static class ConsulConfig<C, S, H>{
        String consulAddr;
        String candidateReportPath;
        String healthStatusPath;
        String replSetConfigPath;
        C candidateType;
        S replsetType;
        H HelathType;

    }
}
