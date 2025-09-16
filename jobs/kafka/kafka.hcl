job "kafka-job" {

  type = "service"
  datacenters = ["*"]

  group "kafka-group" {
    count = 3

    constraint {
      operator = "distinct_hosts"
      value    = "true"
    }


    # task "kafka-prestart" {
    #   env {
    #   }
    #
    #   driver = "raw_exec"
    #   config {
    #     command = "go"
    #     args = ["run", "-C", "${env.CMS_ROOT}\\goctl", ".", "-type", "kafka", "-task", "controller"]
    #   }
    #
    #   lifecycle {
    #     hook = "prestart"
    #     sidecar = false
    #   }
    # }

    task "kafka-task" {

      env {
        JAVA_HOME = "C:\\Users\\ubozkurt\\Downloads\\jdk-24_windows-x64_bin\\jdk-24.0.1"
      }
      # service {
      #   name = "kafka"
      #   port = "broker"
      #   provider = "consul"
      #   check {
      #     type = "tcp"
      #     port = "broker"
      #     interval = "5s"
      #     timeout = "2s"
      #   }
      # }
      resources {
        network {
          port "broker" {static = 9092}
        }
      }

      driver = "raw_exec"
      config {
        command = "go"
        args = ["run", "-C", "${env.CMS_ROOT}\\goctl", ".", "-type", "kafka", "-task", "controller"]
      }

    }

  }
}