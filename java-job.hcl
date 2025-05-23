job "hello-world" {
  # Specifies the datacenter where this job should be run
  # This can be omitted and it will default to ["*"]
  datacenters = ["*"]

  meta {
    # User-defined key/value pairs that can be used in your jobs.
    # You can also use this meta block within Group and Task levels.
    foo = "bar"
  }

  # A group defines a series of tasks that should be co-located
  # on the same client (host). All tasks within a group will be
  # placed on the same host.
  group "servers" {

    # Specifies the number of instances of this group that should be running.
    # Use this to scale or parallelize your job.
    # This can be omitted and it will default to 1.
    count = 1

    service {
      provider = "nomad"
    }

    # Tasks are individual units of work that are run by Nomad.
    task "web" {
      # This particular task starts a simple web server within a Docker container
      driver = "java"

      config {
        class = "com.example.Main"
        class_path    = "C:\\Users\\ubozkurt\\Downloads\\nomad_1.9.4_windows_amd64\\demo\\target\\demo-1.0-SNAPSHOT.jar"
      }

      template {
        data        = <<-EOF
                      @echo off
                      echo starting job
                      echo finish job
                      EOF
        destination = "local/run.cmd"
      }
    }
  }
}