// Copyright 2016-2018 Yubico AB
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#ifndef USB_WINDOWS_H_GUARD
#define USB_WINDOWS_H_GUARD

#include <assert.h>
#include <stdlib.h>
#include <stdio.h>
#include <windows.h>
#include <setupapi.h>
#include <usbiodef.h>
#include <winusb.h>
#include <strsafe.h>

typedef struct DEVICE_CONTEXT
{
    HANDLE deviceHandle;
    WINUSB_INTERFACE_HANDLE usbInterface;

    UCHAR readPipe;
    UCHAR writePipe;

    BOOL initialized;

} DEVICE_CONTEXT, *PDEVICE_CONTEXT;

extern DWORD usbOpen(int vendorId, int productId, char* serialNumber, PDEVICE_CONTEXT* device, ULONG timeout);
extern void  usbClose(PDEVICE_CONTEXT* device);
extern DWORD usbWrite(PDEVICE_CONTEXT device, PUCHAR buffer, ULONG bufferSizeInBytes, PULONG bytesTransferred);
extern DWORD usbRead(PDEVICE_CONTEXT device, PUCHAR buffer, ULONG bufferSizeInBytes, PULONG bytesTransferred);

#endif // USB_WINDOWS_H_GUARD
