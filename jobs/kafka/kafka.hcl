job "kafka-job" {

  type        = "service"
  datacenters = ["*"]

  group "kafka-group" {
    count = 3
    constraint {
      operator = "distinct_hosts"
      value    = "true"
    }

    task "kafka-prestart-task" {

      env {
      }

      driver = "raw_exec"
      config {
        command = "go"
        args    = ["run", "-C", "${env.CMS_ROOT}/goctl", ".", "-kafka-prestart"]
      }

      lifecycle{
        hook = "prestart"
        sidecar = false
      }

    }
    task "kafka-task" {

      env {
      }

      driver = "raw_exec"
      config {
        command = "go"
        args    = ["run", "-C", "${env.CMS_ROOT}/goctl", ".", "-kafka-server"]
      }


    }

  }
}