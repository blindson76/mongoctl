job "mongo-member-job" {

  type = "service"
  datacenters = ["*"]

  group "mongo-member-group" {
    count = 0
    service {
      provider = "nomad"
    }
    constraint {
      attribute = "${meta.role.mongo}"
      value     = "true"
    }
    constraint {
      operator  = "distinct_hosts"
      value     = "true"
    }

    task "mongo-member-task" {
      driver = "java"
      config {
        class = "com.example.MongoWrap"
        class_path    = "C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\demo-1.0-SNAPSHOT.jar;C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\lib\\*"
      }

    }

  }
  group "mongo-control-group" {
    count = 1
    service {
      provider = "nomad"
    }

    task "mongo-control-task" {
      driver = "java"
      config {
        class = "com.example.MongoControl"
        class_path    = "C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\demo-1.0-SNAPSHOT.jar;C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\lib\\*"
      }

    }

  }
}