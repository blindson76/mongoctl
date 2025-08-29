job "mongo-node-job" {

  type = "service"
  datacenters = ["*"]

  group "mongo-node-group" {

    ephemeral_disk {
      migrate = true
      size    = 500
      sticky  = true
    }
    service {
      provider = "nomad"
    }

    task "mongo-node-task" {

      
      driver = "raw_exec"
      config {
          command = "mongod.exe"
          args    = ["--dbpath","${env \"DB_PATH\"}"]
      }
      template {
        data = "mongosecret"
        destination = "${NOMAD_ALLOC_DIR}/keyfile"
      }

    }

  }
}