package com.example;

import com.google.gson.Gson;

import java.io.IOException;
import java.io.InputStreamReader;
import java.net.URI;
import java.net.URISyntaxException;
import java.net.URLConnection;

public class NomadWatch {
    static final String NOMAD_ADDR = System.getenv("NOMAD_ADDR");
    private final String path;
    private String index = "0";
    private String params;
    Gson gson = new Gson();

    public NomadWatch(String path, String... args) {
        this.path = path;
        this.params = String.join("&", args);
        if (args.length > 0) {
            params = params + "&";
        }
    }

    public <T> T watch(Class<T> classOfT) throws IOException, URISyntaxException {
        return gson.fromJson(httpGet(), classOfT);
    }

    private InputStreamReader httpGet() throws IOException, URISyntaxException {
        for (; ; ) {
            try {
                URLConnection con = new URI(String.format("%s/v1/%s?%sindex=%s", NOMAD_ADDR, path, params, index)).toURL().openConnection();
                con.setConnectTimeout(3000);
                con.setReadTimeout(0);
                index = con.getHeaderField("X-Nomad-Index");
                return new InputStreamReader(con.getInputStream());
            } catch (Exception e) {
                try {
                    Thread.sleep(2000);
                } catch (InterruptedException ex) {

                }

            }
        }
    }
}