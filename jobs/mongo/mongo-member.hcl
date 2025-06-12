job "mongo-control-job" {

  type = "service"
  datacenters = ["*"]
  group "mongo-control-group" {
    count = 1
    service {
      provider = "nomad"
    }

    task "mongo-control-task" {
      driver = "java"
      config {
        class = "com.example.MongoControl"
        class_path    = "${env.CMS_ROOT}\\target\\demo-1.0-SNAPSHOT.jar;${env.CMS_ROOT}\\target\\lib\\*"
      }

    }

  }
}