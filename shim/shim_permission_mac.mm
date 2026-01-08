/*
 * shim_permission_mac.mm - macOS permission request implementation
 *
 * Uses AVCaptureDevice API for camera/microphone permission requests.
 * Only compiled on macOS.
 */

#if defined(__APPLE__)

#import <AVFoundation/AVFoundation.h>
#import <dispatch/dispatch.h>
#import <Foundation/Foundation.h>

#define SHIM_EXPORT __attribute__((visibility("default")))

extern "C" {

SHIM_EXPORT int shim_check_camera_permission(void) {
    if (@available(macOS 10.14, *)) {
        AVAuthorizationStatus status = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo];
        switch (status) {
            case AVAuthorizationStatusAuthorized:
                return 1;
            case AVAuthorizationStatusDenied:
            case AVAuthorizationStatusRestricted:
                return 0;
            case AVAuthorizationStatusNotDetermined:
            default:
                return 0;
        }
    }
    // Pre-10.14, permissions are not required
    return 1;
}

SHIM_EXPORT int shim_check_microphone_permission(void) {
    if (@available(macOS 10.14, *)) {
        AVAuthorizationStatus status = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio];
        switch (status) {
            case AVAuthorizationStatusAuthorized:
                return 1;
            case AVAuthorizationStatusDenied:
            case AVAuthorizationStatusRestricted:
                return 0;
            case AVAuthorizationStatusNotDetermined:
            default:
                return 0;
        }
    }
    return 1;
}

SHIM_EXPORT int shim_request_camera_permission(void) {
    if (@available(macOS 10.14, *)) {
        // Check current status first
        AVAuthorizationStatus status = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo];
        if (status == AVAuthorizationStatusAuthorized) {
            return 1;
        }
        if (status == AVAuthorizationStatusDenied || status == AVAuthorizationStatusRestricted) {
            return 0;
        }

        // Request permission - must happen on main thread with run loop
        __block BOOL authorized = NO;
        __block BOOL completed = NO;

        void (^requestBlock)(void) = ^{
            [AVCaptureDevice requestAccessForMediaType:AVMediaTypeVideo completionHandler:^(BOOL granted) {
                authorized = granted;
                completed = YES;
            }];
        };

        if ([NSThread isMainThread]) {
            requestBlock();
        } else {
            dispatch_async(dispatch_get_main_queue(), requestBlock);
        }

        // Spin the run loop until we get a response
        // This allows the permission dialog to be displayed and processed
        NSDate *timeout = [NSDate dateWithTimeIntervalSinceNow:60.0]; // 60 second timeout
        while (!completed && [[NSDate date] compare:timeout] == NSOrderedAscending) {
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
        }

        return authorized ? 1 : 0;
    }
    return 1;
}

SHIM_EXPORT int shim_request_microphone_permission(void) {
    if (@available(macOS 10.14, *)) {
        AVAuthorizationStatus status = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio];
        if (status == AVAuthorizationStatusAuthorized) {
            return 1;
        }
        if (status == AVAuthorizationStatusDenied || status == AVAuthorizationStatusRestricted) {
            return 0;
        }

        __block BOOL authorized = NO;
        __block BOOL completed = NO;

        void (^requestBlock)(void) = ^{
            [AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:^(BOOL granted) {
                authorized = granted;
                completed = YES;
            }];
        };

        if ([NSThread isMainThread]) {
            requestBlock();
        } else {
            dispatch_async(dispatch_get_main_queue(), requestBlock);
        }

        // Spin the run loop until we get a response
        NSDate *timeout = [NSDate dateWithTimeIntervalSinceNow:60.0];
        while (!completed && [[NSDate date] compare:timeout] == NSOrderedAscending) {
            [[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.1]];
        }

        return authorized ? 1 : 0;
    }
    return 1;
}

}  // extern "C"

#endif  // __APPLE__
