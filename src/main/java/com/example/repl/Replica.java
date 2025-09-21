package com.example.repl;

import com.google.gson.Gson;
import org.bson.conversions.Bson;

public interface Replica<C extends Replica.CandidateReportType,S extends Replica.ReplicaSetSpecType,H extends Replica.HealthStatusType> {
    public interface Unique {
        String GetId();
    }
    public abstract  class ReplicaSetSpecType<S> extends JSONSerializable<S>{

        protected ReplicaSetSpecType(Class<S> type) {
            super(type);
        }
    }

    public abstract class HealthStatusType<H> extends JSONSerializable<H> implements Unique{

        protected HealthStatusType(Class<H> type) {
            super(type);
        }
    }

    public abstract class CandidateReportType<C> extends JSONSerializable<C> implements Unique{

        protected CandidateReportType(Class<C> type) {
            super(type);
        }
    }
    public abstract class JSONSerializable<T> {
        private transient static final Gson gson = new Gson();
        private transient final Class<T> clazz;
        protected JSONSerializable(Class<T> type){
            this.clazz = type;
        }

        public T Parse(String data) {
            return gson.fromJson(data, clazz);

        }
        public String JSONBytes(){
            return  gson.toJson(this);
        }
    }
}
