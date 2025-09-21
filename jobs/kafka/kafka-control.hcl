
job "kafka-control-job" {

  type = "service"
  datacenters = ["*"]

  group "kafka-control-group" {

    count = 1


    task "kafka-control-task" {

      
      driver = "raw_exec"
      config {
          command = "go"
          args    = ["run", "-C", "${env.CMS_ROOT}\\goctl", ".", "-type","kafka","-task","controller","-jobFile","${env.CMS_ROOT}/jobs/kafka/kafka.hcl"]
      }

    }

  }
}

  