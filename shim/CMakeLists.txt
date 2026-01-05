cmake_minimum_required(VERSION 3.16)
project(libwebrtc_shim VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# Options
option(USE_SYSTEM_LIBWEBRTC "Use system-installed libwebrtc" OFF)
option(BUILD_SHARED_LIBS "Build shared library" ON)

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
)

# libwebrtc library
if(APPLE)
    set(LIBWEBRTC_LIBRARY ${LIBWEBRTC_DIR}/lib/libwebrtc.a)
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

# Shim library sources
set(SHIM_SOURCES
    shim.cc
)

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

if(APPLE)
    target_compile_definitions(webrtc_shim PRIVATE WEBRTC_MAC)
elseif(UNIX)
    target_compile_definitions(webrtc_shim PRIVATE WEBRTC_LINUX)
elseif(WIN32)
    target_compile_definitions(webrtc_shim PRIVATE WEBRTC_WIN NOMINMAX)
endif()

# Link libraries
target_link_libraries(webrtc_shim
    PRIVATE
        ${LIBWEBRTC_LIBRARY}
        ${PLATFORM_LIBS}
)

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

install(FILES shim.h DESTINATION include)

# Print configuration
message(STATUS "")
message(STATUS "libwebrtc_shim configuration:")
message(STATUS "  Version:      ${PROJECT_VERSION}")
message(STATUS "  Build type:   ${CMAKE_BUILD_TYPE}")
message(STATUS "  Shared lib:   ${BUILD_SHARED_LIBS}")
message(STATUS "  libwebrtc:    ${LIBWEBRTC_LIBRARY}")
message(STATUS "")
