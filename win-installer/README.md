# Description

Source for the Windows installer for the yubihsm-connector

# Prerequisities

 * WiX v3.9+ (obtained from [wixtoolset.org/releases](http://wixtoolset.org/releases))
 * Visual Studio 2015 or MSBuild that can build WiX projects

# Build

 * Add nssm.exe (from [nssm.cc](http://nssm.cc)) to the yubihsm-connector project's **bin/** folder
 * Open the `YubiHSMConnectorInstaller.sln` solution file
 * Set the active configuration to **_Release X64_**
 * Build
 * You can find the msi build output in the **x64/** folder
