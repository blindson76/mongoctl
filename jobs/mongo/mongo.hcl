job "mongo-service-job" {

  type = "system"
  datacenters = ["*"]

  group "mongo-service-group" {

    ephemeral_disk {
      migrate = true
      size    = 500
      sticky  = true
    }
    service {
      provider = "nomad"
    }

    task "mongo-service-task" {

      
      driver = "java"
      config {
          class = "com.example.MongoService"
          class_path    = "${env.CMS_ROOT}\\target\\demo-1.0-SNAPSHOT.jar;${env.CMS_ROOT}\\target\\lib\\*"
      }
      template {
        data = "mongosecret"
        destination = "${NOMAD_ALLOC_DIR}/keyfile"
      }

    }

  }
}