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
          class_path    = "C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\demo-1.0-SNAPSHOT.jar;C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\lib\\*"
      }
      template {
        data = "mongosecret"
        destination = "${NOMAD_ALLOC_DIR}/keyfile"
      }

    }

  }
}