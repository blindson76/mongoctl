
job "mongo-control-job" {

  type = "service"
  datacenters = ["*"]

  group "mongo-control-group" {

    count = 1


    task "mongo-control-task" {

      
      driver = "raw_exec"
      config {
          command = "go"
          args    = ["run", "-C", "${env.CMS_ROOT}\\goctl", ".", "-controller"]
      }

    }

  }
}

  