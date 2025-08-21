@echo off

set wait_time=%1

rem Set up signal handling for CTRL+C
rem This will run an infinite loop until interrupted
:loop
timeout /t 1 /nobreak >nul 2>&1
if errorlevel 1 goto loop

rem If we reach here, we were interrupted
rem Wait for the specified time using Windows timeout command
timeout /t %wait_time% /nobreak >nul 2>&1

rem Exit with the wait time as status code
exit /b %wait_time%