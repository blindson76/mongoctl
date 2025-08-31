package main

import (
	"log"
	"os"
)

func mongoWrapper() {
	log.Println("start mongo wrapper tash")
	for _, k := range os.Environ() {
		log.Println(k, os.Getenv(k))
	}
}
