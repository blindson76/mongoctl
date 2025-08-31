
variable "replica-count" {
    type = number
    default = 1
}
variable "replica-members" {
    type = string
    default = ""
}

job "mongo-service-job" {

  type = "service"
  datacenters = ["*"]

  group "mongo-service-group" {

    count = var.replica-count

    constraint {
        attribute = "${node.unique.name}"
        operator = "regexp"
        value = var.replica-members
    }
    
    constraint {
        distinct_hosts = true
    }

    task "mongo-service-task" {

      
      driver = "raw_exec"
      config {
          command = "go"
          args    = ["run", "-C", "${env.CMS_ROOT}\\goctl", ".", "-mongo"]
      }

    }

  }
}

  