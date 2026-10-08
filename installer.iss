; Script generated for Inno Setup 6
#define MyAppName "TaxSolution"
#define MyAppVersion "1.0.0"
#define MyAppPublisher "ACC Online"
#define MyAppURL "https://github.com/HTrongThang/tax-solution"
#define MyAppExeName "Frontend.exe"

[Setup]
; AppId duy nhất để nhận diện app khi nâng cấp phiên bản mới
AppId={{8B842F55-6E95-4BD9-9A01-1C34B8FA22E0}}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
; Tự động đóng ứng dụng nếu đang chạy để cập nhật mượt mà
CloseApplications=yes
CloseApplicationsFilter=Frontend.exe,TaxBot.exe
; Icon bộ cài đặt
SetupIconFile=Frontend\Properties\app.ico
; Nén tối đa để file cài đặt nhẹ nhất
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
OutputDir=output_installer
OutputBaseFilename=TaxSolution_Setup_v{#MyAppVersion}
; Cho phép cài đặt không cần quyền Admin (phù hợp máy công ty/kế toán)
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: checkedonce

[Files]
; Sao chép toàn bộ file từ thư mục dist đã build
Source: "dist\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\Frontend.exe"
Name: "{group}\Gỡ cài đặt {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon; IconFilename: "{app}\Frontend.exe"

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent
