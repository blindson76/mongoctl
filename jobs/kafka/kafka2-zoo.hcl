job "kafka2-zoo-control-job" {

  type = "service"
  datacenters = ["*"]

  group "kafka2-zoo-control-group" {
    count = 3
    constraint {
      attribute = "${meta.mongo.role}"
      value = "true"
    }
    constraint {
      operator  = "distinct_hosts"
      value     = "true"
    }

    task "kafka2-zookeeper-task" {

      env {
        CLUSTER_ID           = "b8f7d0e2-6ff6-4c8d-a4e2-6d8a4db54edb" # use your generated ID
        JAVA_HOME						 = "C:\\Program Files\\Java\\jdk-23"
        NODE_ID							 = "${NOMAD_ALLOC_INDEX}"
      }

      driver = "raw_exec"
      config {
        command = "cmd.exe"
        args = ["/c", "${env.CMS_ROOT}\\cots\\kf\\bin\\windows\\zookeeper-server-start.bat", "${NOMAD_TASK_DIR}\\config\\zookeeper.properties" ]
      }

      template {
        data = <<EOF
# Directory to store ZooKeeper data (snapshots and logs)
dataDir={{ env "KAFKA_DATA_DIR" }}/zookeeper

# Port on which ZooKeeper listens for client connections
clientPort=2181
clientPortAddress={{ env "CSB_IP" }}

# Basic timing config (leave default)
tickTime=2000
initLimit=10
syncLimit=5

server.3=10.72.84.54:2888:3888
server.4=10.72.84.56:2888:3888
server.5=10.72.84.58:2888:3888


EOF
        destination = "local/config/zookeeper.properties"

      }

    }

  }
}