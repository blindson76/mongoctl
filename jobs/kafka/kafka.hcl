
variable "replica-count" {
  type = number
  default = 1
}
variable "replica-members" {
  type = string
  default = ""
}
job "kafka-job" {

  type = "service"
  datacenters = ["*"]

  group "kafka-group" {
    count = var.replica-count

    constraint {
      operator = "distinct_hosts"
      value    = "true"
    }
    constraint {
      attribute = "${node.unique.name}"
      operator = "set_contains_any"
      value = var.replica-members
    }

    task "kafka-task" {

      env {
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
        args = ["run", "-C", "${env.CMS_ROOT}\\goctl", ".", "-type", "kafka", "-task", "member"]
      }

    }

  }
}