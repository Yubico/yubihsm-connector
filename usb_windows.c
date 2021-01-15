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

#include "usb_windows.h"

const DWORD PIPE_WRITE = 0x01;
const DWORD PIPE_READ  = 0x81;

#define safe_close_handle(X) if (X != INVALID_HANDLE_VALUE) { CloseHandle(X); }
#define safe_free(X) if (X) { free(X); }
#define safe_free_usb(X) if (X != INVALID_HANDLE_VALUE) { WinUsb_Free(X); }

// For some reason, CGO isn't finding this declaration, despite all of the correct include
// files and compiler flags being set (that I can think of). Copying the declaration here
// allows us to work around this issue.
WINBASEAPI
int
WINAPI
CompareStringOrdinal(
    LPCWCH lpString1,
    _In_ int cchCount1,
    LPCWCH lpString2,
    _In_ int cchCount2,
    _In_ BOOL bIgnoreCase
    );


static DWORD GetDeviceInterfaceDetails(
    HDEVINFO deviceInfoSet,
    PSP_DEVINFO_DATA deviceInfoData,
    PSP_DEVICE_INTERFACE_DETAIL_DATA* deviceDetails
    )
{
    SP_DEVICE_INTERFACE_DATA         deviceInterfaceData = { sizeof(SP_INTERFACE_DEVICE_DATA) };
    PSP_DEVICE_INTERFACE_DETAIL_DATA deviceInterfaceDetailData = NULL;
    DWORD                            error               = ERROR_SUCCESS;
    ULONG                            requiredBytes       = 0;

    assert(deviceInfoSet);
    assert(deviceInfoData);
    assert(deviceDetails);

    if (!SetupDiEnumDeviceInterfaces(deviceInfoSet,
                                     deviceInfoData,
                                     &GUID_DEVINTERFACE_USB_DEVICE,
                                     0,
                                     &deviceInterfaceData))
    {
        error = GetLastError();
        printf("SetupDiEnumDeviceInterfaces failed with 0x%x\n", error);
        goto Cleanup;
    }

    // First call gets the size of the buffer needed. We expect it to fail with
    // ERROR_INSUFFICIENT_BUFFER.
    if (!SetupDiGetDeviceInterfaceDetail(deviceInfoSet,
                                         &deviceInterfaceData,
                                         NULL,
                                         0,
                                         &requiredBytes,
                                         NULL))
    {
        error = GetLastError();
        if (error != ERROR_INSUFFICIENT_BUFFER)
        {
            printf("SetupDiGetDeviceInterfaceDetails(1) failed with 0x%x\n", error);
            goto Cleanup;
        }
        error = ERROR_SUCCESS;
    }
    else
    {
        // We SHOULD have failed with ERROR_INSUFFICIENT_BUFFER, but we didn't. No
        // sense in continuing. Fail with a generic failure code.
        assert(FALSE);
        error = ERROR_GEN_FAILURE;
        goto Cleanup;
    }

    assert(requiredBytes >= sizeof(SP_DEVICE_INTERFACE_DETAIL_DATA));

    deviceInterfaceDetailData = malloc(requiredBytes);
    if (!deviceInterfaceDetailData)
    {
        error = ERROR_OUTOFMEMORY;
        goto Cleanup;
    }

    deviceInterfaceDetailData->cbSize = sizeof(SP_DEVICE_INTERFACE_DETAIL_DATA);

    // Since we should have the exact buffer size now, this call should succeed and
    // give us the device interface details (the device instance path).
    if (!SetupDiGetDeviceInterfaceDetail(deviceInfoSet,
                                         &deviceInterfaceData,
                                         deviceInterfaceDetailData,
                                         requiredBytes,
                                         NULL,
                                         NULL))
    {
        error = GetLastError();
        printf("SetupDiGetDeviceInterfaceDetails(2) failed with 0x%x\n", error);
        goto Cleanup;
    }

    *deviceDetails = deviceInterfaceDetailData;
    deviceInterfaceDetailData = NULL;

Cleanup:
    safe_free(deviceInterfaceDetailData);
    return error;
}

static DWORD GetDeviceHandle(PTSTR devicePath, HANDLE* deviceHandle)
{
    HANDLE devHandle = INVALID_HANDLE_VALUE;

    assert(devicePath);
    assert(deviceHandle);

    devHandle = CreateFile(devicePath,
                           GENERIC_READ | GENERIC_WRITE,
                           FILE_SHARE_READ | FILE_SHARE_WRITE,
                           NULL,
                           OPEN_EXISTING,
                           FILE_FLAG_OVERLAPPED, // WinUsb requires overlapped I/O
                           NULL);

    if (devHandle == INVALID_HANDLE_VALUE)
    {
        printf("CreateFile failed with 0x%x\n", GetLastError());
        return GetLastError();
    }

    *deviceHandle = devHandle;
    return ERROR_SUCCESS;
}

static BOOL IsMatchingDevice(WINUSB_INTERFACE_HANDLE interfaceHandle, int vendorId, int productId, char* serialNumber)
{
    ULONG                  bytesTransferred = 0;
    USB_DEVICE_DESCRIPTOR  deviceDescriptor = { 0 };
    DWORD                  error            = ERROR_SUCCESS;
    BYTE                   serialBuffer[MAXIMUM_USB_STRING_LENGTH] = { 0 }; // Maximum size of descriptor is 255 (struct size field is only a byte)
    PUSB_STRING_DESCRIPTOR serialDescriptor = NULL;
    ULONG                  serialLength     = 0;
    WCHAR                  serialNumberBuffer[MAXIMUM_USB_STRING_LENGTH] = { 0 };
    WCHAR                  wideSerialNumber[MAXIMUM_USB_STRING_LENGTH] = { 0 };

    assert(interfaceHandle && (interfaceHandle != INVALID_HANDLE_VALUE));

    // The USB_DEVICE_DESCRIPTOR contains the VID and PID, along with the descriptor
    // number for the device's serial number.
    if (!WinUsb_GetDescriptor(interfaceHandle,
                              USB_DEVICE_DESCRIPTOR_TYPE,
                              0,
                              0x409, // English
                              (PUCHAR)&deviceDescriptor,
                              sizeof(deviceDescriptor),
                              &bytesTransferred))
    {
        error = GetLastError();
        printf("WinUsb_GetDescriptor(DEVICE) failed with 0x%x\n", error);
        goto Cleanup;
    }

    // Regardless of whether a serial number was defined - we need to make sure this
    // is at least the same kind of device we're looking for. Do these checks first
    // as they're cheap.
    if ((deviceDescriptor.idVendor != (USHORT)vendorId) ||
        (deviceDescriptor.idProduct != (USHORT)productId))
    {
        error = ERROR_OBJECT_NOT_FOUND;
        goto Cleanup;
    }

    // If the user provided a serialNumber, we need to grab the string descriptor which
    // contains the string version of the serialNumber and compare the values after normalizing
    // both on UTF-16.
    if (serialNumber)
    {
        ZeroMemory(serialBuffer, sizeof(serialBuffer));

        if (!WinUsb_GetDescriptor(interfaceHandle,
                                  USB_STRING_DESCRIPTOR_TYPE,
                                  deviceDescriptor.iSerialNumber,
                                  0x409, // English
                                  (PUCHAR)serialBuffer,
                                  sizeof(serialBuffer),
                                  &bytesTransferred))
        {
            error = GetLastError();
            printf("WinUsb_GetDescriptor(STRING) failed with 0x%x\n", error);
            goto Cleanup;
        }

        // USB Strings are UTF-16LE. The length is 2 less than the whole descriptor
        // length, as per the USB spec (9.6.9 in USB 3.2 spec). Maximum size is 255.
        serialDescriptor = (PUSB_STRING_DESCRIPTOR)serialBuffer;
        serialLength = serialDescriptor->bLength - 2;

        // The USB spec is a bit ambiguous as to whether if a string descriptor is present
        // that it MUST contain string data. (9.5 and 9.6.9 in USB 3.2 spec) However, since
        // we've already matched against a Yubico device, we support serial numbers, so we
        // should assert that the serial number length is non-zero.
        assert(serialLength > 0);

        // We're being called from Go, which means the incoming serialNumber is actually
        // UTF-8 and not ASCII. Convert to UTF-16 so we can compare using CompareStringOrdinal.
        if (!MultiByteToWideChar(CP_UTF8, 0, serialNumber, -1, wideSerialNumber, 255))
        {
            error = GetLastError();
            printf("MultiByteToWideChar failed with 0x%x\n", error);
            goto Cleanup;
        }

        // Copy the device's serial number into a temporary buffer. StrinchCchCopyN will guarantee
        // that the copy is null-terminated, which let's us do a slightly easier string comparison below.
        if (FAILED(StringCchCopyN(serialNumberBuffer, 255, serialDescriptor->bString, serialLength)))
        {
            error = ERROR_INVALID_PARAMETER;
            printf("StringCchCopyN failed.");
            goto Cleanup;
        }

        // wideSerialNumber is the UTF-16 conversion of Go's serial number string. It's
        // guaranteed to be null-terminated. serialNumberBuffer is also guaranteed to
        // be null terminated (see above). Because of this, we'll let CompareStringOrdinal
        // find the string lengths as it sees fit (-1 as parameter)
        if (CompareStringOrdinal(wideSerialNumber,
                                 -1,
                                 serialNumberBuffer,
                                 -1,
                                 TRUE /* case insensitive */) != CSTR_EQUAL)
        {
            error = ERROR_OBJECT_NOT_FOUND;
            printf("CompareStringOrdinal failed with 0x%x\n", error);
            printf("WideSerialNumber: %S\n", wideSerialNumber);
            printf("Descriptor: %S\n", serialNumberBuffer);

            goto Cleanup;
        }
    }

Cleanup:
    return error == ERROR_SUCCESS;
}

static DWORD GetUsbDevice(int vendorId, int productId, char* serialNumber, PDEVICE_CONTEXT ctx, ULONG confTimeout)
{
    HANDLE                           deviceHandle    = INVALID_HANDLE_VALUE;
    SP_DEVINFO_DATA                  deviceInfoData  = { sizeof(SP_DEVINFO_DATA) };
    HDEVINFO                         deviceInfoSet   = INVALID_HANDLE_VALUE;
    PSP_DEVICE_INTERFACE_DETAIL_DATA deviceInterfaceDetailData = NULL;
    DWORD                            error           = ERROR_SUCCESS;
    WINUSB_INTERFACE_HANDLE          interfaceHandle = INVALID_HANDLE_VALUE;

    assert(ctx);

    // Find all USB Devices (excluding roots, hubs, and hid devices)
    deviceInfoSet = SetupDiGetClassDevs(&GUID_DEVINTERFACE_USB_DEVICE,
                                        NULL, NULL,
                                        DIGCF_PRESENT | DIGCF_DEVICEINTERFACE);

    if (deviceInfoSet == INVALID_HANDLE_VALUE)
    {
        error = GetLastError();
        printf("SetupDiGetClassDevs failed with 0x%x\n", error);
        goto Cleanup;
    }

    // Iterate through each device found, open it, and determine if it's the device we're looking for.
    // Errors here are treated as best effort. If we encounter a failure, simply clean up and continue
    // on to the next device.
    for (int index = 0; SetupDiEnumDeviceInfo(deviceInfoSet, index, &deviceInfoData); index++)
    {
        error = ERROR_SUCCESS;
        safe_free(deviceInterfaceDetailData);
        safe_close_handle(deviceHandle);
        safe_free_usb(interfaceHandle);

        error = GetDeviceInterfaceDetails(deviceInfoSet, &deviceInfoData, &deviceInterfaceDetailData);
        if (error == ERROR_NO_MORE_ITEMS)
        {
            // We've hit the end of our device list. We didn't find anything, so set the error
            // code to something more appropriate and exit.
            error = ERROR_OBJECT_NOT_FOUND;
            goto Cleanup;
        }
        else if (error != ERROR_SUCCESS)
        {
            // This is the one case where we do not skip the device. There's no reason SetupDi should
            // fail, so we don't want to get stuck in an infinite loop here. Better to just exit.
            goto Cleanup;
        }

        error = GetDeviceHandle(deviceInterfaceDetailData->DevicePath, &deviceHandle);
        if (error != ERROR_SUCCESS)
        {
            continue;
        }

        if (!WinUsb_Initialize(deviceHandle, &interfaceHandle))
        {
            error = GetLastError();
            interfaceHandle = INVALID_HANDLE_VALUE; // It isn't documented what interfaceHandle's value
                                                    // would be on failure, so explicitly set it so we
                                                    // can deterministically clean up if needed.

            // Only report interesting errors.
            if (error != ERROR_NOT_SUPPORTED && error != ERROR_SHARING_VIOLATION)
            {
                printf("WinUsb_Initialize failed with 0x%x\n", error);
            }
            continue;
        }

        if (!IsMatchingDevice(interfaceHandle, vendorId, productId, serialNumber))
        {
            // Set an error in case this is the last iteration of the loop.
            error = ERROR_OBJECT_NOT_FOUND;
            continue;
        }

        {
            ULONG timeout = confTimeout;
            if (!WinUsb_SetPipePolicy(interfaceHandle, PIPE_READ, PIPE_TRANSFER_TIMEOUT,
                    sizeof(timeout), &timeout)) {
                error = GetLastError();
                continue;
            }

            if (!WinUsb_SetPipePolicy(interfaceHandle, PIPE_WRITE, PIPE_TRANSFER_TIMEOUT,
                    sizeof(timeout), &timeout)) {
                error = GetLastError();
                continue;
            }

            // This is vitally important since it declares that ZLP should be sent when a message
            // would otherwise end on a packet boundary.
            if (!WinUsb_SetPipePolicy(interfaceHandle, PIPE_WRITE,
                    SHORT_PACKET_TERMINATE, 1, (PVOID) "\x1")) {
                error = GetLastError();
                continue;
            }
        }

        // Device found, break out of loop and return it.
        break;
    }
    if (error != ERROR_SUCCESS) {
        // We exited the loop above while in an error state, we need to clean up.
        goto Cleanup;
    }

    ctx->deviceHandle = deviceHandle;
    ctx->usbInterface = interfaceHandle;
    ctx->readPipe     = PIPE_READ;
    ctx->writePipe    = PIPE_WRITE;
    ctx->initialized  = TRUE;

    deviceHandle    = INVALID_HANDLE_VALUE;
    interfaceHandle = INVALID_HANDLE_VALUE;

Cleanup:
    safe_free(deviceInterfaceDetailData);
    safe_close_handle(deviceHandle);
    safe_free_usb(interfaceHandle);

    if (deviceInfoSet != INVALID_HANDLE_VALUE)
    {
        SetupDiDestroyDeviceInfoList(deviceInfoSet);
    }
    return error;
}

DWORD usbOpen(int vendorId, int productId, char* serialNumber, PDEVICE_CONTEXT* device, ULONG timeout)
{
    PDEVICE_CONTEXT ctx   = NULL;
    DWORD           error = ERROR_SUCCESS;

    if (!device)
    {
        error = ERROR_INVALID_PARAMETER;
        goto Cleanup;
    }

    *device = NULL;

    ctx = (PDEVICE_CONTEXT)malloc(sizeof(*ctx));
    if (!ctx)
    {
        error = ERROR_OUTOFMEMORY;
        goto Cleanup;
    }

    ctx->deviceHandle = INVALID_HANDLE_VALUE;
    ctx->usbInterface = INVALID_HANDLE_VALUE;
    ctx->readPipe     = 0;
    ctx->writePipe    = 0;
    ctx->initialized  = FALSE;

    error = GetUsbDevice(vendorId, productId, serialNumber, ctx, timeout);
    if (error != ERROR_SUCCESS)
    {
        printf("GetUsbDevice returned 0x%x\n", error);
        goto Cleanup;
    }

    *device = ctx;
    ctx = NULL;

Cleanup:
    if (ctx)
    {
        if (ctx->deviceHandle != INVALID_HANDLE_VALUE)
        {
            CloseHandle(ctx->deviceHandle);
        }
        if (ctx->usbInterface != INVALID_HANDLE_VALUE)
        {
            WinUsb_Free(ctx->usbInterface);
        }
    }

    return error;
}

void usbClose(PDEVICE_CONTEXT* device)
{
    PDEVICE_CONTEXT deref = NULL;

    if (!device)
    {
        return;
    }

    deref = *device;

    if (!deref)
    {
        return;
    }

    if (deref->initialized)
    {
        WinUsb_Free(deref->usbInterface);
        CloseHandle(deref->deviceHandle);
        deref->initialized = FALSE;
    }

    free(deref);
    *device = NULL;
}

DWORD usbCheck(PDEVICE_CONTEXT device, int vendorId, int productId)
{
    if (!IsMatchingDevice(device->usbInterface, vendorId, productId, NULL))
    {
        return ERROR_OBJECT_NOT_FOUND;
    }
    return ERROR_SUCCESS;
}

DWORD usbWrite(PDEVICE_CONTEXT device, PUCHAR buffer, ULONG bufferSizeInBytes, PULONG bytesTransferred)
{
    if (!device || !device->initialized)
    {
        return ERROR_INVALID_STATE;
    }

    if (!buffer || bufferSizeInBytes == 0)
    {
        return ERROR_INVALID_PARAMETER;
    }

    if (!WinUsb_WritePipe(device->usbInterface,
                          device->writePipe,
                          buffer,
                          bufferSizeInBytes,
                          bytesTransferred,
                          NULL))
    {
        return GetLastError();
    }

    return ERROR_SUCCESS;
}

DWORD usbRead(PDEVICE_CONTEXT device, PUCHAR buffer, ULONG bufferSizeInBytes, PULONG bytesTransferred)
{
    if (!device || !device->initialized)
    {
        return ERROR_INVALID_STATE;
    }

    if (!buffer || !bytesTransferred || bufferSizeInBytes == 0)
    {
        return ERROR_INVALID_PARAMETER;
    }

    if (!WinUsb_ReadPipe(device->usbInterface,
                         device->readPipe,
                         buffer,
                         bufferSizeInBytes,
                         bytesTransferred,
                         NULL))
    {
        return GetLastError();
    }

    return ERROR_SUCCESS;
}
