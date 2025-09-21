package com.example.repl;

import java.util.List;

public interface Store <C,S,H> {
    void PutCandidateReport(String id, C val);
    void WatchCandidateReports(Listener<List<C>> listener);
    void UpdateHealthStatus(String id, H status);
    void WatchHealthStatus(Listener<List<H>> listener);
    void UpdateReplSetConfig(S spec);
    void WatchReplSetConfig(Listener<List<S>> listener);
    public interface Listener<C>{
        void AcceptData(C data);
    }

}
