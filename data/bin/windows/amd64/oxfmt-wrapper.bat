@echo off
setlocal enabledelayedexpansion

REM oxfmt-wrapper.bat - Windows 等价于 oxfmt-wrapper (bash)
REM 包装 oxfmt 以支持 stdin/stdout 格式化
REM 用法: echo "code" | oxfmt-wrapper.bat js

set "ext=%~1"

REM --version 支持
if "%ext%"=="--version" (
    set "script_dir=%~dp0"
    if exist "!script_dir!oxfmt.exe" (
        "!script_dir!oxfmt.exe" --version
    ) else (
        where oxfmt >nul 2>&1
        if !errorlevel! equ 0 (oxfmt --version) else (echo oxfmt-wrapper 1.0.0 ^(oxfmt not installed^))
    )
    exit /b 0
)

if "%ext%"=="" set "ext=js"

REM 创建临时目录
set "tmpdir=%TEMP%\oxfmt_%RANDOM%"
mkdir "%tmpdir%" 2>nul
set "tmpfile=%tmpdir%\input.%ext%"

REM 从 stdin 读取到临时文件
more > "%tmpfile%"

REM 在脚本同目录查找 oxfmt
set "script_dir=%~dp0"
set "oxfmt_bin=!script_dir!oxfmt.exe"
if not exist "!oxfmt_bin!" (
    where oxfmt >nul 2>&1
    if !errorlevel! equ 0 (set "oxfmt_bin=oxfmt") else (
        echo Error: oxfmt not found >&2
        rd /s /q "%tmpdir%" 2>nul
        exit /b 1
    )
)

REM oxfmt 原地格式化
"%oxfmt_bin%" --write "%tmpfile%" >nul 2>&1
if !errorlevel! neq 0 (
    echo Error: oxfmt format failed >&2
    rd /s /q "%tmpdir%" 2>nul
    exit /b 1
)

REM 输出格式化结果
type "%tmpfile%"

REM 清理
rd /s /q "%tmpdir%" 2>nul
