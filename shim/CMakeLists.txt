cmake_minimum_required(VERSION 3.16)
project(libwebrtc_shim VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# Options
option(USE_SYSTEM_LIBWEBRTC "Use system-installed libwebrtc" OFF)
option(BUILD_SHARED_LIBS "Build shared library" ON)
option(ENABLE_DEVICE_CAPTURE "Enable camera/mic/screen capture support" ON)

# Find libwebrtc
# Users should set LIBWEBRTC_DIR to point to their libwebrtc build
if(NOT LIBWEBRTC_DIR)
    if(DEFINED ENV{LIBWEBRTC_DIR})
        set(LIBWEBRTC_DIR $ENV{LIBWEBRTC_DIR})
    else()
        message(FATAL_ERROR "LIBWEBRTC_DIR not set. Point it to your libwebrtc build directory.")
    endif()
endif()

message(STATUS "Using libwebrtc from: ${LIBWEBRTC_DIR}")

# libwebrtc include directories
set(LIBWEBRTC_INCLUDE_DIRS
    ${LIBWEBRTC_DIR}/include
    ${LIBWEBRTC_DIR}/include/third_party/abseil-cpp
    ${LIBWEBRTC_DIR}/include/third_party/boringssl/src/include
    ${LIBWEBRTC_DIR}/include/third_party/libyuv/include
    ${LIBWEBRTC_DIR}/include/third_party/opus/src/include
    ${LIBWEBRTC_DIR}/include/build
)

# libwebrtc library
if(APPLE)
    set(LIBWEBRTC_LIBRARY ${LIBWEBRTC_DIR}/lib/libwebrtc.a)
    # Additional codec factory libraries (separate from libwebrtc.a)
    set(LIBWEBRTC_BUILTIN_ENCODER ${LIBWEBRTC_DIR}/lib/libbuiltin_video_encoder_factory.a)
    set(LIBWEBRTC_BUILTIN_DECODER ${LIBWEBRTC_DIR}/lib/libbuiltin_video_decoder_factory.a)
    set(LIBWEBRTC_INTERNAL_CODECS ${LIBWEBRTC_DIR}/lib/librtc_internal_video_codecs.a)
    set(LIBWEBRTC_SIMULCAST_ADAPTER ${LIBWEBRTC_DIR}/lib/librtc_simulcast_encoder_adapter.a)
    set(LIBWEBRTC_FALLBACK_WRAPPERS ${LIBWEBRTC_DIR}/lib/librtc_software_fallback_wrappers.a)
    set(PLATFORM_LIBS
        "-framework Foundation"
        "-framework AVFoundation"
        "-framework CoreAudio"
        "-framework CoreMedia"
        "-framework CoreVideo"
        "-framework VideoToolbox"
        "-framework AudioToolbox"
        "-framework CoreFoundation"
        "-framework CoreGraphics"
        "-framework IOSurface"
        "-framework Metal"
        "-framework AppKit"
        "-framework ScreenCaptureKit"
        "-framework ApplicationServices"
    )
elseif(UNIX)
    set(LIBWEBRTC_LIBRARY ${LIBWEBRTC_DIR}/lib/libwebrtc.a)
    set(PLATFORM_LIBS
        pthread
        dl
        rt
    )
elseif(WIN32)
    set(LIBWEBRTC_LIBRARY ${LIBWEBRTC_DIR}/lib/webrtc.lib)
    set(PLATFORM_LIBS
        winmm
        ws2_32
        secur32
        iphlpapi
        crypt32
        dmoguids
        wmcodecdspuuid
        amstrmid
        msdmo
        strmiids
    )
endif()

# Check if library exists
if(NOT EXISTS ${LIBWEBRTC_LIBRARY})
    message(FATAL_ERROR "libwebrtc library not found at: ${LIBWEBRTC_LIBRARY}")
endif()

# Shim library sources (modular)
set(SHIM_SOURCES
    shim_common.cc
    shim_video_codec.cc
    shim_audio_codec.cc
    shim_packetizer.cc
    shim_peer_connection.cc
    shim_rtp_sender.cc
    shim_rtp_receiver.cc
    shim_rtp_transceiver.cc
    shim_track_source.cc
    shim_data_channel.cc
    shim_capture.cc
    shim_remote_sink.cc
    shim_stats.cc
)

# Platform-specific permission sources
if(APPLE)
    list(APPEND SHIM_SOURCES shim_permission_mac.mm)
else()
    list(APPEND SHIM_SOURCES shim_permission.cc)
endif()

# Create the shared library
add_library(webrtc_shim ${SHIM_SOURCES})

target_include_directories(webrtc_shim
    PUBLIC
        ${CMAKE_CURRENT_SOURCE_DIR}
    PRIVATE
        ${LIBWEBRTC_INCLUDE_DIRS}
)

# Compile definitions
target_compile_definitions(webrtc_shim PRIVATE
    WEBRTC_POSIX
    WEBRTC_LIBRARY_IMPL
)

if(ENABLE_DEVICE_CAPTURE)
    target_compile_definitions(webrtc_shim PRIVATE SHIM_ENABLE_DEVICE_CAPTURE)
endif()

if(APPLE)
    target_compile_definitions(webrtc_shim PRIVATE WEBRTC_MAC)
elseif(UNIX)
    target_compile_definitions(webrtc_shim PRIVATE WEBRTC_LINUX)
elseif(WIN32)
    target_compile_definitions(webrtc_shim PRIVATE WEBRTC_WIN NOMINMAX)
endif()

# Link libraries
# Order matters: specific libraries first, then libwebrtc.a, then frameworks
if(APPLE AND EXISTS ${LIBWEBRTC_BUILTIN_ENCODER} AND EXISTS ${LIBWEBRTC_BUILTIN_DECODER})
    set(ADDITIONAL_LIBS "")
    if(EXISTS ${LIBWEBRTC_INTERNAL_CODECS})
        list(APPEND ADDITIONAL_LIBS ${LIBWEBRTC_INTERNAL_CODECS})
    endif()
    if(EXISTS ${LIBWEBRTC_SIMULCAST_ADAPTER})
        list(APPEND ADDITIONAL_LIBS ${LIBWEBRTC_SIMULCAST_ADAPTER})
    endif()
    if(EXISTS ${LIBWEBRTC_FALLBACK_WRAPPERS})
        list(APPEND ADDITIONAL_LIBS ${LIBWEBRTC_FALLBACK_WRAPPERS})
    endif()
    target_link_libraries(webrtc_shim
        PRIVATE
            ${LIBWEBRTC_BUILTIN_ENCODER}
            ${LIBWEBRTC_BUILTIN_DECODER}
            ${ADDITIONAL_LIBS}
            ${LIBWEBRTC_LIBRARY}
            ${PLATFORM_LIBS}
    )
else()
    target_link_libraries(webrtc_shim
        PRIVATE
            ${LIBWEBRTC_LIBRARY}
            ${PLATFORM_LIBS}
    )
endif()

# Set output name
set_target_properties(webrtc_shim PROPERTIES
    OUTPUT_NAME "webrtc_shim"
    VERSION ${PROJECT_VERSION}
    SOVERSION 1
)

# Visibility settings
if(BUILD_SHARED_LIBS)
    set_target_properties(webrtc_shim PROPERTIES
        CXX_VISIBILITY_PRESET hidden
        VISIBILITY_INLINES_HIDDEN ON
    )
endif()

# Install
install(TARGETS webrtc_shim
    LIBRARY DESTINATION lib
    ARCHIVE DESTINATION lib
    RUNTIME DESTINATION bin
)

install(FILES shim.h shim_common.h DESTINATION include)

# Print configuration
message(STATUS "")
message(STATUS "libwebrtc_shim configuration:")
message(STATUS "  Version:        ${PROJECT_VERSION}")
message(STATUS "  Build type:     ${CMAKE_BUILD_TYPE}")
message(STATUS "  Shared lib:     ${BUILD_SHARED_LIBS}")
message(STATUS "  Device capture: ${ENABLE_DEVICE_CAPTURE}")
message(STATUS "  libwebrtc:      ${LIBWEBRTC_LIBRARY}")
message(STATUS "")
