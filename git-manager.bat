@echo off
chcp 65001 >nul
setlocal enableextensions enabledelayedexpansion

REM ============================================================
REM  Git Manager - UI for common Git operations
REM  Tags, branches, commits, push/pull - all in one menu.
REM
REM  This file is in .gitignore - DO NOT COMMIT IT.
REM ============================================================

cd /d "%~dp0"

:menu
cls
echo.
echo  ============================================================
echo                     GIT MANAGER
echo  ============================================================
echo.
echo   BRANCH:  [%BRANCH%]   REMOTE: [%REMOTE%]
echo   LATEST:  [%LATEST_TAG%]
echo.
echo  ------------------------------------------------------------
echo   WORKING WITH COMMITS:
echo     1.  Status + diff summary
echo     2.  Add all ^& commit (with message)
echo     3.  Add all ^& commit + tag (release)
echo     4.  Amend last commit (keep tag)
echo.
echo   TAGS:
echo     5.  List all tags (sorted)
echo     6.  Create tag on current commit
echo     7.  Delete tag (local ^& remote)
echo     8.  Push all tags to remote
echo.
echo   BRANCHES:
echo     9.  List branches
echo    10.  Switch branch
echo    11.  Create new branch
echo    12.  Delete branch
echo    13.  Merge branch into current
echo.
echo   SYNC:
echo    14.  Push current branch + tags
echo    15.  Pull current branch
echo    16.  Fetch all (branches + tags)
echo    17.  Force push (DANGER)
echo.
echo   UTILS:
echo    18.  Full log (last 20 commits)
echo    19.  Show what changed in last commit
echo    20.  Undo last commit (keep files)
echo    21.  Hard reset to HEAD (DISCARD ALL CHANGES!)
echo    22.  Init fresh repo (only if no .git)
echo.
echo    0.  Exit
echo  ============================================================
echo.

REM Get current branch and latest tag for display
for /f "delims=" %%b in ('git rev-parse --abbrev-ref HEAD 2^>nul') do set "BRANCH=%%b"
for /f "delims=" %%t in ('git describe --tags --abbrev^=0 2^>nul') do set "LATEST_TAG=%%t"
for /f "delims=" %%r in ('git remote 2^>nul') do set "REMOTE=%%r"
if "%BRANCH%"=="" set "BRANCH=NO-GIT"
if "%REMOTE%"=="" set "REMOTE=none"

set /p "choice=Select [0-22]: "

if "%choice%"=="0" goto :eof
if "%choice%"=="1" goto status
if "%choice%"=="2" goto commit
if "%choice%"=="3" goto commit_tag
if "%choice%"=="4" goto amend
if "%choice%"=="5" goto tags_list
if "%choice%"=="6" goto tag_create
if "%choice%"=="7" goto tag_delete
if "%choice%"=="8" goto tags_push
if "%choice%"=="9" goto branches_list
if "%choice%"=="10" goto branch_switch
if "%choice%"=="11" goto branch_create
if "%choice%"=="12" goto branch_delete
if "%choice%"=="13" goto branch_merge
if "%choice%"=="14" goto push_all
if "%choice%"=="15" goto pull
if "%choice%"=="16" goto fetch
if "%choice%"=="17" goto force_push
if "%choice%"=="18" goto log_full
if "%choice%"=="19" goto last_diff
if "%choice%"=="20" goto undo_commit
if "%choice%"=="21" goto hard_reset
if "%choice%"=="22" goto init_repo

goto menu

REM ============================================================
REM 1. STATUS
REM ============================================================
:status
cls
echo.
echo  === STATUS ===
echo.
git status -sb
echo.
echo  --- UNTRACKED FILES ---
echo.
git ls-files --others --exclude-standard
echo.
pause
goto menu

REM ============================================================
REM 2. COMMIT (add all + message)
REM ============================================================
:commit
cls
echo.
set /p "msg=Commit message: "
if "!msg!"=="" (
  echo Empty message, abort.
  pause
  goto menu
)
git add -A
git commit -m "!msg!"
echo.
pause
goto menu

REM ============================================================
REM 3. COMMIT + TAG (release)
REM ============================================================
:commit_tag
cls
echo.
set /p "ver=Version (e.g. 1.4.37): "
if "!ver!"=="" (
  echo Empty version, abort.
  pause
  goto menu
)
set /p "msg=Commit message [release: !ver!]: "
if "!msg!"=="" set "msg=release: !ver!"
git add -A
git commit -m "!msg!"
git tag v!ver!
echo.
echo  Created commit + tag v!ver!
echo.
pause
goto menu

REM ============================================================
REM 4. AMEND
REM ============================================================
:amend
cls
echo.
echo  Current last commit:
git log -1 --oneline
echo.
set /p "msg=New message (empty = keep old): "
git add -A
if "!msg!"=="" (
  git commit --amend --no-edit
) else (
  git commit --amend -m "!msg!"
)
echo.
pause
goto menu

REM ============================================================
REM 5. LIST TAGS
REM ============================================================
:tags_list
cls
echo.
echo  === ALL TAGS (latest first) ===
echo.
git tag --sort=-version:refname
echo.
echo  --- LATEST TAG INFO ---
git describe --tags --abbrev=0 2>nul
echo.
pause
goto menu

REM ============================================================
REM 6. CREATE TAG
REM ============================================================
:tag_create
cls
echo.
set /p "ver=Tag name (e.g. v1.4.37): "
if "!ver!"=="" (
  echo Empty, abort.
  pause
  goto menu
)
set /p "anno=Annotated message (empty = lightweight): "
if "!anno!"=="" (
  git tag !ver!
) else (
  git tag -a !ver! -m "!anno!"
)
echo.
echo  Tag !ver! created.
echo.
pause
goto menu

REM ============================================================
REM 7. DELETE TAG
REM ============================================================
:tag_delete
cls
echo.
echo  Existing tags:
git tag --sort=-version:refname
echo.
set /p "ver=Tag to delete: "
if "!ver!"=="" goto menu
git tag -d !ver!
echo  Delete from remote too? (y/n)
set /p "yn=[n]: "
if /i "!yn!"=="y" git push origin :refs/tags/!ver!
echo.
pause
goto menu

REM ============================================================
REM 8. PUSH ALL TAGS
REM ============================================================
:tags_push
cls
echo.
echo  Pushing all tags to origin...
git push origin --tags
echo.
pause
goto menu

REM ============================================================
REM 9. LIST BRANCHES
REM ============================================================
:branches_list
cls
echo.
echo  === LOCAL BRANCHES ===
git branch
echo.
echo  === REMOTE BRANCHES ===
git branch -r
echo.
pause
goto menu

REM ============================================================
REM 10. SWITCH BRANCH
REM ============================================================
:branch_switch
cls
echo.
echo  Local branches:
git branch
echo.
set /p "name=Branch name to switch to: "
if "!name!"=="" goto menu
git checkout !name!
echo.
pause
goto menu

REM ============================================================
REM 11. CREATE BRANCH
REM ============================================================
:branch_create
cls
echo.
set /p "name=New branch name: "
if "!name!"=="" goto menu
git checkout -b !name!
echo.
pause
goto menu

REM ============================================================
REM 12. DELETE BRANCH
REM ============================================================
:branch_delete
cls
echo.
echo  Local branches (except current):
git branch
echo.
set /p "name=Branch to delete: "
if "!name!"=="" goto menu
git branch -d !name!
echo  Also delete from remote? (y/n)
set /p "yn=[n]: "
if /i "!yn!"=="y" git push origin --delete !name!
echo.
pause
goto menu

REM ============================================================
REM 13. MERGE BRANCH
REM ============================================================
:branch_merge
cls
echo.
echo  Current branch:
git rev-parse --abbrev-ref HEAD
echo.
echo  Available branches:
git branch
echo.
set /p "name=Branch to merge INTO current: "
if "!name!"=="" goto menu
git merge !name!
echo.
pause
goto menu

REM ============================================================
REM 14. PUSH ALL (branch + tags)
REM ============================================================
:push_all
cls
echo.
echo  Pushing current branch + tags...
for /f "delims=" %%b in ('git rev-parse --abbrev-ref HEAD') do set "B=%%b"
git push origin !B!
git push origin --tags
echo.
pause
goto menu

REM ============================================================
REM 15. PULL
REM ============================================================
:pull
cls
echo.
for /f "delims=" %%b in ('git rev-parse --abbrev-ref HEAD') do set "B=%%b"
echo  Pulling !B!...
git pull origin !B!
echo.
pause
goto menu

REM ============================================================
REM 16. FETCH ALL
REM ============================================================
:fetch
cls
echo.
echo  Fetching all remotes + tags...
git fetch --all --tags
echo.
pause
goto menu

REM ============================================================
REM 17. FORCE PUSH
REM ============================================================
:force_push
cls
echo.
echo  !!! WARNING: FORCE PUSH !!!
echo  This rewrites remote history. Type YES to confirm.
echo.
set /p "confirm=Type YES: "
if not "!confirm!"=="YES" goto menu
for /f "delims=" %%b in ('git rev-parse --abbrev-ref HEAD') do set "B=%%b"
git push origin !B! --force
echo.
pause
goto menu

REM ============================================================
REM 18. FULL LOG
REM ============================================================
:log_full
cls
echo.
echo  === LAST 20 COMMITS ===
echo.
git log --oneline --graph --decorate -20
echo.
pause
goto menu

REM ============================================================
REM 19. LAST DIFF
REM ============================================================
:last_diff
cls
echo.
echo  === CHANGES IN LAST COMMIT ===
echo.
git show --stat HEAD
echo.
echo  Show full diff? (y/n)
set /p "yn=[n]: "
if /i "!yn!"=="y" git show HEAD
echo.
pause
goto menu

REM ============================================================
REM 20. UNDO LAST COMMIT (keep files)
REM ============================================================
:undo_commit
cls
echo.
echo  Current last commit:
git log -1 --oneline
echo.
echo  Undo it? Files stay changed. (y/n)
set /p "yn=[n]: "
if /i not "!yn!"=="y" goto menu
git reset --soft HEAD~1
echo.
echo  Done. Changes are staged.
pause
goto menu

REM ============================================================
REM 21. HARD RESET
REM ============================================================
:hard_reset
cls
echo.
echo  !!! DANGER: HARD RESET !!!
echo  This DISCARDS ALL uncommitted changes (forever).
echo.
set /p "confirm=Type DELETE to confirm: "
if not "!confirm!"=="DELETE" goto menu
git reset --hard HEAD
git clean -fd
echo.
echo  Done. Working tree is clean.
pause
goto menu

REM ============================================================
REM 22. INIT REPO
REM ============================================================
:init_repo
cls
echo.
if exist ".git" (
  echo  .git already exists! Aborting.
  pause
  goto menu
)
set /p "url=Remote URL (empty = no remote): "
git init
if not "!url!"=="" (
  git remote add origin !url!
  git add -A
  git commit -m "initial commit"
  git branch -M main
  echo.
  echo  Repo initialized. Push now? (y/n)
  set /p "yn=[n]: "
  if /i "!yn!"=="y" git push -u origin main
)
echo.
pause
goto menu
