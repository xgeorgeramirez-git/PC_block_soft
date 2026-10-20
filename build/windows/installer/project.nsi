Unicode true

####
## Установщик PC Block: свой шаблон NSIS вместо шаблона Wails по умолчанию
## (план M1, задача T15; ТЗ FR-1, FR-4, FR-26, 8.3.1, 8.3.8; Ruling 21).
##
## Wails берёт этот файл, пока он лежит в build/windows/installer. Файл wails_tools.nsh Wails
## пишет заново при каждой сборке из своего встроенного шаблона, поэтому править его бесполезно
## и в репозитории его нет: всё своё живёт здесь.
##
## Установка. Прежняя версия, если есть, останавливается (PCBlock.exe --remove-service, данные
## остаются). Потом файлы, деинсталлятор, служба (PCBlock.exe --install-service), ярлык сброса
## пароля в меню Пуск. Без /S открывается мастер первой настройки (PCBlock.exe --wizard).
##
## Удаление. Сначала вопрос программе: PCBlock.exe --uninstall-gate. Её код выхода читает
## установщик: 0 удалять, статистику оставить; 2 удалять вместе со статистикой; 1 отмена.
## Программа не настроена или службы нет: пароль не нужен, это решает сама программа.
## Потом PCBlock.exe --remove-service (служба, процессы, запрет Диспетчера задач, ключ SafeBoot)
## и удаление файлов. Папка данных в ProgramData удаляется только при коде 2.
##
## Тихо: setup.exe /S и uninstall.exe /S. Тихое удаление настроенной программы читает
## /PASSWORD=пароль родителя и /DELETEDATA (удалить и статистику).
##
## Правила файла: UTF-8 с BOM, иначе кириллица бьётся; комментарий не заканчивается обратной
## косой чертой, иначе NSIS съедает следующую строку. Проверка: tools/check_nsis_bom.sh.
####
!include "wails_tools.nsh"

# Версия в четырёх частях.
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

# HiDPI.
ManifestDPIAware true

!include "MUI2.nsh"
!include "LogicLib.nsh"
!insertmacro un.GetParameters
!insertmacro un.GetOptions

# Тексты, которые Wails по умолчанию пишет по-английски.
!define WAILS_WIN10_REQUIRED "PC Block работает в Windows 10 (Windows Server 2016) и новее."
!define WAILS_ARCHITECTURE_NOT_SUPPORTED "PC Block нельзя установить на эту версию Windows. Поддерживается: ${ARCH}."
!define WAILS_INSTALL_WEBVIEW_DETAILPRINT "Установка: WebView2 Runtime"

# Ярлык сброса пароля родителя (FR-26): имя дословно из ТЗ.
!define PCB_RESET_LINK "$SMPROGRAMS\${INFO_PRODUCTNAME} — сброс пароля родителя.lnk"

# 1: при удалении убрать и папку данных (программа ответила кодом 2).
Var PcbDeleteData

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_TEXT "PC Block установлен. Мастер первой настройки открыт в отдельном окне: задайте пароль родителя и отметьте детские учётные записи. Ограничения начнут действовать при следующем входе детей."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Russian"

VIAddVersionKey /LANG=${LANG_RUSSIAN} "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey /LANG=${LANG_RUSSIAN} "FileDescription" "${INFO_PRODUCTNAME}: установщик"
VIAddVersionKey /LANG=${LANG_RUSSIAN} "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey /LANG=${LANG_RUSSIAN} "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey /LANG=${LANG_RUSSIAN} "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey /LANG=${LANG_RUSSIAN} "ProductName"     "${INFO_PRODUCTNAME}"

Name "${INFO_PRODUCTNAME}"
BrandingText "${INFO_PRODUCTNAME} ${INFO_PRODUCTVERSION}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\PCBlock"
ShowInstDetails show
ShowUninstDetails show

# Запускает PCBlock.exe из папки установки с ключами ARGS, ждёт его и кладёт в $0 код выхода.
# Программу не удалось запустить: $0 равен 99. Ключи без кавычек и пробелов в значениях.
!macro pcb.exec ARGS
    ClearErrors
    ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" ${ARGS}' $0
    ${If} ${Errors}
    ${OrIf} $0 == "error"
        StrCpy $0 99
    ${EndIf}
!macroend

Function .onInit
    !insertmacro wails.checkArchitecture

    # Установка поверх: берём папку прежней установки, если папку не задали ключом /D=.
    SetRegView 64
    ${If} $INSTDIR == "$PROGRAMFILES64\PCBlock"
        ReadRegStr $0 HKLM "${UNINST_KEY}" "InstallLocation"
        ${If} $0 != ""
        ${AndIf} ${FileExists} "$0\${PRODUCT_EXECUTABLE}"
            StrCpy $INSTDIR $0
        ${EndIf}
    ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    # Установка поверх (Ruling 21): служба и процессы в сеансах держат PCBlock.exe, пока их не
    # остановили. Данные не трогаем: ключ --delete-data не передаётся.
    ${If} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        DetailPrint "Останавливаю прежнюю версию PC Block"
        !insertmacro pcb.exec "--remove-service --silent"
        DetailPrint "Прежняя версия остановлена, код выхода $0"
    ${EndIf}

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortCut "${PCB_RESET_LINK}" "$INSTDIR\${PRODUCT_EXECUTABLE}" "--reset-password" "$INSTDIR\${PRODUCT_EXECUTABLE}" 0
    # Ярлыки шаблона Wails прежних версий: на общем рабочем столе их видят и дети.
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    # Деинсталлятор пишем до службы: если служба не поднимется, программу можно удалить.
    !insertmacro wails.writeUninstaller
    SetRegView 64
    WriteRegStr HKLM "${UNINST_KEY}" "InstallLocation" "$INSTDIR"

    # Служба: регистрация, автозапуск, перезапуск при сбое, запуск. Повторный запуск безопасен.
    # Сразу после остановки прежней версии Windows бывает не готова пересоздать службу, поэтому три попытки.
    StrCpy $R9 0
    ${Do}
        DetailPrint "Регистрирую и запускаю службу PC Block"
        !insertmacro pcb.exec "--install-service --silent"
        ${If} $0 = 0
            ${ExitDo}
        ${EndIf}
        IntOp $R9 $R9 + 1
        ${If} $R9 >= 3
            ${ExitDo}
        ${EndIf}
        Sleep 3000
    ${Loop}
    ${If} $0 <> 0
        ${If} ${Silent}
            SetErrorLevel 3
        ${Else}
            MessageBox MB_OK|MB_ICONSTOP "Не удалось зарегистрировать и запустить службу PC Block (код $0).$\r$\nЗапустите установку ещё раз. Если не поможет, перезагрузите компьютер и повторите."
        ${EndIf}
        Abort "Служба PC Block не запущена"
    ${EndIf}

    # Мастер первой настройки: из этого же процесса, он уже с правами администратора.
    # Тихая установка (/S) мастер не открывает.
    ${IfNot} ${Silent}
        Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}" --wizard'
    ${EndIf}
SectionEnd

Function un.onInit
    StrCpy $PcbDeleteData 0

    # Программы нет (удалили руками): спрашивать некого.
    ${IfNot} ${FileExists} "$INSTDIR\${PRODUCT_EXECUTABLE}"
        Return
    ${EndIf}

    # Вопрос программе. Окно спросит пароль родителя (или подтверждение администратора,
    # если пароль забыт) и галочку "Удалить статистику". Тихо: параметры из командной строки.
    ${If} ${Silent}
        ${un.GetParameters} $R0
        ClearErrors
        ${un.GetOptions} $R0 "/PASSWORD=" $R1
        ${If} ${Errors}
            StrCpy $R1 ""
        ${EndIf}
        StrCpy $R2 ""
        ClearErrors
        ${un.GetOptions} $R0 "/DELETEDATA" $R3
        ${IfNot} ${Errors}
            StrCpy $R2 " --delete-data"
        ${EndIf}
        ClearErrors
        ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --uninstall-gate --silent "--password=$R1"$R2' $0
    ${Else}
        ClearErrors
        ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --uninstall-gate' $0
    ${EndIf}
    ${If} ${Errors}
    ${OrIf} $0 == "error"
        StrCpy $0 99
    ${EndIf}

    # Код выхода: 0 удалять, 2 удалять вместе со статистикой, остальное (1 отмена) прерывает удаление.
    ${If} $0 == 2
        StrCpy $PcbDeleteData 1
    ${ElseIf} $0 <> 0
        ${If} $0 = 99
        ${AndIfNot} ${Silent}
            MessageBox MB_OK|MB_ICONEXCLAMATION "Не удалось проверить разрешение на удаление PC Block. Попробуйте ещё раз."
        ${EndIf}
        SetErrorLevel 1
        Abort
    ${EndIf}
FunctionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # Служба, сеансные процессы, запрет Диспетчера задач у детей, ключ SafeBoot. Данные в
    # ProgramData программа удаляет сама, если ей передан --delete-data.
    StrCpy $R9 0
    ${Do}
        DetailPrint "Останавливаю службу PC Block и снимаю правила"
        ${If} $PcbDeleteData = 1
            !insertmacro pcb.exec "--remove-service --silent --delete-data"
        ${Else}
            !insertmacro pcb.exec "--remove-service --silent"
        ${EndIf}
        ${If} $0 = 0
            ${ExitDo}
        ${EndIf}
        IntOp $R9 $R9 + 1
        ${If} $R9 >= 2
            ${ExitDo}
        ${EndIf}
        Sleep 3000
    ${Loop}
    ${If} $0 <> 0
        # Часть защиты осталась включённой. Файлы не трогаем, чтобы удаление можно было повторить.
        ${If} ${Silent}
            SetErrorLevel 4
        ${Else}
            MessageBox MB_OK|MB_ICONSTOP "Службу PC Block или правила для детей не удалось снять полностью (код $0).$\r$\nПрограмма осталась на месте. Перезагрузите компьютер и повторите удаление."
        ${EndIf}
        Abort "Служба PC Block не удалена"
    ${EndIf}

    # Папка данных нужна только до удаления; запасной путь на случай, если программа её не убрала.
    ${If} $PcbDeleteData = 1
        RMDir /r "$APPDATA\PCBlock"
    ${EndIf}

    Delete "${PCB_RESET_LINK}"
    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    # Данные WebView2 текущего пользователя.
    SetShellVarContext current
    RMDir /r "$APPDATA\${PRODUCT_EXECUTABLE}"
    SetShellVarContext all

    Delete /REBOOTOK "$INSTDIR\${PRODUCT_EXECUTABLE}"
    !insertmacro wails.deleteUninstaller
    RMDir /REBOOTOK "$INSTDIR"
SectionEnd
