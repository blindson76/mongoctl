package com.example.repl.mongo;

import lombok.Builder;
import lombok.NoArgsConstructor;

public class MongoManager {
    MongoConfig cfg;
    @Builder
    public static class MongoConfig{
        String localAddr;
        String localPort;
        String addr;
        String port;
        String dbPath;
        String rsName;
        String keyFile;
    }

}
