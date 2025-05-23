package com.example;

public class Main {
    public static void main(String[] args) {
        
        new Thread(()->{
            while (true) {
                System.out.println("deneme123");
                try {
                    Thread.sleep(2000);
                } catch (InterruptedException e) {
                    // TODO Auto-generated catch block
                    e.printStackTrace();
                }
            }
        })
        .start();
    }
}