@echo off
rem rmdir /s /q %temp%\mongo%1
mkdir %temp%\mongo%1
set /a port=27010 + %1
start "mongo-%1" /min cots\mongo\mongod --dbpath %temp%\mongo%1 --replSet rs0 --bind_ip 10.10.11.1 --port %PORT%