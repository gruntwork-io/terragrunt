@echo off

set wait_time=%1

rem Simple infinite loop that can be interrupted
:loop
rem Use ping to create a 1-second delay (ping localhost -n 2 creates ~1 second delay)
ping -n 2 127.0.0.1 >nul 2>&1
goto loop