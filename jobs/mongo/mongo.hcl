job "mongo-job" {

  type = "sysbatch"
  datacenters = ["*"]

  group "mongo-group" {

    service {
      provider = "mongo"
    }

    task "mongo-prestart" {

      lifecycle {
        hook = "prestart"
        sidecar = false
      }
      
      driver = "java"
      config {
          class = "com.example.MongoPrestart"
          class_path    = "C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\demo-1.0-SNAPSHOT.jar"
      }

    }

    task "mongo" {

      driver = "java"

      config {
        class = "com.example.Mongo"
        class_path    = "C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\demo-1.0-SNAPSHOT.jar"
      }
    }
  }
}