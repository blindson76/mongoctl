@echo off
set "PATH=C:\Users\ubozkurt\Downloads\jdk-24_windows-x64_bin\jdk-24.0.1\bin;%PATH%"
set "JAVA_HOME=C:\Users\ubozkurt\Downloads\jdk-24_windows-x64_bin\jdk-24.0.1"
taskkill -f -im notepad.exe
taskkill /f /im nomad.exe
taskkill /f /fi "WINDOWTITLE eq Node-*" 
for %%l in (1 2 3 6) do (
    start cmd /c start-agent.bat %%l
)