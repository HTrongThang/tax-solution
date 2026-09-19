; TaxSolution Installer Script (InnoSetup)
; Download InnoSetup tai: https://jrsoftware.org/isdl.php
; Mo file nay bang InnoSetup -> Compile -> ra file Setup_TaxSolution.exe

[Setup]
AppName=TaxSolution
AppVersion=1.0
DefaultDirName={autopf}\TaxSolution
DefaultGroupName=TaxSolution
OutputBaseFilename=Setup_TaxSolution
OutputDir=dist
Compression=lzma2/ultra64
SolidCompression=yes
SetupIconFile=Frontend\Properties\app.ico 
UninstallDisplayIcon={app}\Frontend.exe

[Files]
Source: "dist\TaxSolution\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs

[Icons]
Name: "{group}\TaxSolution"; Filename: "{app}\Frontend.exe"; IconFilename: "{app}\Frontend.exe"
Name: "{commondesktop}\TaxSolution"; Filename: "{app}\Frontend.exe"; IconFilename: "{app}\Frontend.exe"

[Run]
Filename: "{app}\Frontend.exe"; Description: "Chay TaxSolution"; Flags: nowait postinstall
