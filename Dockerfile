# libgowebrtc CI Dockerfile
#
# This Dockerfile supports three modes:
#
# 1. Use pre-built libwebrtc (FAST - mount your local build):
#    docker build --build-arg SKIP_LIBWEBRTC_BUILD=1 -t libgowebrtc-ci .
#    docker run -v ~/libwebrtc:/opt/libwebrtc libgowebrtc-ci
#
# 2. Use cached libwebrtc image (MODERATE - uses Docker cache):
#    docker build -t libgowebrtc-ci .
#    # First build is slow, subsequent builds use cache
#
# 3. Force rebuild libwebrtc (SLOW - 2-3 hours):
#    docker build --no-cache -t libgowebrtc-ci .
#
# Running tests:
#   docker run --rm libgowebrtc-ci make test
#   docker run --rm libgowebrtc-ci go test -v ./pkg/...
#
# Interactive:
#   docker run -it --rm libgowebrtc-ci bash

# ==============================================================================
# Stage 1: libwebrtc builder (cached separately)
# ==============================================================================
FROM ubuntu:22.04 AS libwebrtc-builder

ARG SKIP_LIBWEBRTC_BUILD=0
ARG WEBRTC_BRANCH=branch-heads/7390
ARG TARGET_CPU=x64
ARG ENABLE_H264=1

ENV DEBIAN_FRONTEND=noninteractive

# Install build dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    python3 \
    python3-pip \
    pkg-config \
    curl \
    wget \
    gnupg \
    clang \
    lld \
    ninja-build \
    rsync \
    lsb-release \
    software-properties-common \
    sudo \
    libglib2.0-dev \
    libasound2-dev \
    libpulse-dev \
    libx11-dev \
    libxcomposite-dev \
    libxdamage-dev \
    libxext-dev \
    libxfixes-dev \
    libxrandr-dev \
    libxtst-dev \
    libxrender-dev \
    libdrm-dev \
    libgbm-dev \
    libegl1-mesa-dev \
    libgl1-mesa-dev \
    && rm -rf /var/lib/apt/lists/*

# Install a modern clang toolchain for Linux arm64 (WebRTC expects LLVM 21).
RUN if [ "$TARGET_CPU" = "arm64" ]; then \
        curl -fsSL https://apt.llvm.org/llvm.sh -o /tmp/llvm.sh && \
        chmod +x /tmp/llvm.sh && \
        /tmp/llvm.sh 21 && \
        rm -f /tmp/llvm.sh; \
    fi

# Ensure clang builtins are available at the path expected by Chromium build files.
RUN if [ "$TARGET_CPU" = "arm64" ]; then \
        clang_lib_dir="/usr/lib/llvm-21/lib/clang/21/lib" && \
        if [ -f "${clang_lib_dir}/linux/libclang_rt.builtins-aarch64.a" ]; then \
            mkdir -p "${clang_lib_dir}/aarch64-unknown-linux-gnu" && \
            ln -sf "${clang_lib_dir}/linux/libclang_rt.builtins-aarch64.a" \
                "${clang_lib_dir}/aarch64-unknown-linux-gnu/libclang_rt.builtins.a"; \
        fi; \
    fi

# Set up depot_tools
RUN git clone --depth=1 https://chromium.googlesource.com/chromium/tools/depot_tools.git /opt/depot_tools
ENV PATH="/opt/depot_tools:$PATH"

# Build libwebrtc (skip if SKIP_LIBWEBRTC_BUILD=1)
WORKDIR /build
ENV BUILD_DIR=/build/webrtc
ENV INSTALL_DIR=/opt/libwebrtc

# Create placeholder if skipping build
RUN if [ "$SKIP_LIBWEBRTC_BUILD" = "1" ]; then \
        mkdir -p /opt/libwebrtc/lib /opt/libwebrtc/include && \
        echo "PLACEHOLDER - mount real libwebrtc at /opt/libwebrtc" > /opt/libwebrtc/PLACEHOLDER; \
    fi

# Fetch and build WebRTC (only if not skipping)
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        mkdir -p ${BUILD_DIR} && \
        echo 'solutions = [{"name": "src", "url": "https://webrtc.googlesource.com/src.git@'${WEBRTC_BRANCH}'", "deps_file": "DEPS", "managed": False}]' > ${BUILD_DIR}/.gclient && \
        echo 'target_os = ["linux"]' >> ${BUILD_DIR}/.gclient && \
        cd ${BUILD_DIR} && gclient sync --nohooks --no-history; \
    fi

RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        cd ${BUILD_DIR}/src && gclient runhooks; \
    fi

RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        python3 -c "from pathlib import Path; path = Path('/build/webrtc/src/build/config/compiler/BUILD.gn'); needle = 'cflags += [ \"-Wa,--crel,--allow-experimental-crel\" ]'; text = path.read_text(encoding='utf-8'); path.write_text(text.replace(needle, '# ' + needle), encoding='utf-8') if needle in text else None"; \
    fi

# Generate build configuration
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        mkdir -p ${BUILD_DIR}/src/out/Release && \
        echo 'is_debug = false' > ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'is_component_build = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'rtc_include_tests = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'rtc_build_examples = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        if [ "$ENABLE_H264" = "1" ]; then \
            echo 'proprietary_codecs = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
            echo 'rtc_use_h264 = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
            echo 'ffmpeg_branding = "Chrome"' >> ${BUILD_DIR}/src/out/Release/args.gn; \
        else \
            echo 'proprietary_codecs = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
            echo 'rtc_use_h264 = false' >> ${BUILD_DIR}/src/out/Release/args.gn; \
        fi && \
        echo 'rtc_include_dav1d_in_internal_decoder_factory = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'use_rtti = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'use_custom_libcxx = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'use_clang_modules = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'treat_warnings_as_errors = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'target_os = "linux"' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo "target_cpu = \"${TARGET_CPU}\"" >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'is_clang = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        if [ "${TARGET_CPU}" = "arm64" ]; then \
            echo 'clang_base_path = "/usr/lib/llvm-21"' >> ${BUILD_DIR}/src/out/Release/args.gn && \
            echo 'clang_version = "21"' >> ${BUILD_DIR}/src/out/Release/args.gn && \
            echo 'clang_use_chrome_plugins = false' >> ${BUILD_DIR}/src/out/Release/args.gn; \
        fi && \
        echo 'use_sysroot = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        cd ${BUILD_DIR}/src && gn gen out/Release; \
    fi

# Build libwebrtc (this is the slowest part)
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        cd ${BUILD_DIR}/src && ninja -C out/Release -j$(nproc) \
            :webrtc \
            api/video_codecs:builtin_video_decoder_factory \
            api/video_codecs:builtin_video_encoder_factory \
            api/video_codecs:rtc_software_fallback_wrappers \
            api/audio_codecs:builtin_audio_decoder_factory \
            api/audio_codecs:builtin_audio_encoder_factory \
            media:rtc_internal_video_codecs \
            media:rtc_simulcast_encoder_adapter; \
    fi

# Install libwebrtc
# Note: We only copy libwebrtc.a which is a regular archive. The codec factory
# libraries are thin archives that can't be easily copied outside the build tree.
# We build the shim here while the build tree is available.
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        mkdir -p ${INSTALL_DIR}/include ${INSTALL_DIR}/lib && \
        cp ${BUILD_DIR}/src/out/Release/obj/libwebrtc.a ${INSTALL_DIR}/lib/; \
    fi

# Build shim while build tree is available (thin archives need source references)
ARG TARGETARCH=amd64
COPY shim /workspace/shim
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        apt-get update && apt-get install -y cmake && rm -rf /var/lib/apt/lists/* && \
        if [ "${TARGET_CPU}" = "arm64" ]; then \
            SHIM_C_COMPILER="/usr/lib/llvm-21/bin/clang"; \
            SHIM_CXX_COMPILER="/usr/lib/llvm-21/bin/clang++"; \
            SHIM_TOOLCHAIN_FLAGS="-rtlib=compiler-rt"; \
        else \
            SHIM_C_COMPILER="clang"; \
            SHIM_CXX_COMPILER="clang++"; \
            SHIM_TOOLCHAIN_FLAGS=""; \
        fi && \
        mkdir -p /workspace/shim/build && cd /workspace/shim/build && \
        cmake .. \
            -DCMAKE_BUILD_TYPE=Release \
            -DLIBWEBRTC_DIR=${BUILD_DIR}/src/out/Release \
            -DBUILD_SHARED_LIBS=ON \
            -DCMAKE_C_COMPILER="${SHIM_C_COMPILER}" \
            -DCMAKE_CXX_COMPILER="${SHIM_CXX_COMPILER}" \
            -DCMAKE_C_FLAGS="${SHIM_TOOLCHAIN_FLAGS}" \
            -DCMAKE_CXX_FLAGS="${SHIM_TOOLCHAIN_FLAGS}" \
            -DCMAKE_SHARED_LINKER_FLAGS="-fuse-ld=lld ${SHIM_TOOLCHAIN_FLAGS}" && \
        cmake --build . --config Release -j$(nproc) && \
        mkdir -p ${INSTALL_DIR}/lib/linux_${TARGETARCH} && \
        cp libwebrtc_shim.so ${INSTALL_DIR}/lib/linux_${TARGETARCH}/; \
    fi

# Copy headers
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        cd ${BUILD_DIR}/src && \
        for dir in api call common_video media modules p2p pc rtc_base system_wrappers common_audio video audio logging stats sdk; do \
            if [ -d "$dir" ]; then \
                rsync -av --include='*.h' --include='*/' --exclude='*' "$dir/" ${INSTALL_DIR}/include/"$dir"/ 2>/dev/null || true; \
            fi; \
        done && \
        mkdir -p ${INSTALL_DIR}/include/third_party/abseil-cpp && \
        rsync -av --include='*.h' --include='*.inc' --include='*/' --exclude='*' \
            third_party/abseil-cpp/absl/ ${INSTALL_DIR}/include/third_party/abseil-cpp/absl/ 2>/dev/null || true; \
    fi

# ==============================================================================
# Stage 2: Runtime environment
# ==============================================================================
FROM ubuntu:22.04 AS runtime

ENV DEBIAN_FRONTEND=noninteractive

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    cmake \
    pkg-config \
    curl \
    clang \
    lld \
    libasound2 \
    libpulse0 \
    libx11-6 \
    libxcomposite1 \
    libxdamage1 \
    libxfixes3 \
    libxext6 \
    libxrandr2 \
    libxrender1 \
    libxtst6 \
    libdrm2 \
    libgbm1 \
    libglib2.0-0 \
    libegl1 \
    libgl1 \
    && rm -rf /var/lib/apt/lists/*

# Install Go
ARG GO_VERSION=1.25.3
ARG TARGETARCH
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${TARGETARCH:-amd64}.tar.gz" | tar -C /usr/local -xzf -
ENV PATH="/usr/local/go/bin:$PATH"
ENV GOPATH="/go"
ENV PATH="$GOPATH/bin:$PATH"

# Copy libwebrtc from builder
COPY --from=libwebrtc-builder /opt/libwebrtc /opt/libwebrtc
ENV LIBWEBRTC_DIR=/opt/libwebrtc

# Set up workspace
WORKDIR /workspace

# Copy go.mod first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Copy pre-built shim from builder stage (built with full build tree access)
# The shim is built in the builder stage to access thin archives from the build tree.
ARG TARGETARCH=amd64
RUN mkdir -p /workspace/lib/linux_${TARGETARCH}
COPY --from=libwebrtc-builder /opt/libwebrtc/lib/linux_${TARGETARCH}/libwebrtc_shim.so /workspace/lib/linux_${TARGETARCH}/

# Set library path
ENV LIBWEBRTC_SHIM_PATH=/workspace/lib/linux_${TARGETARCH}/libwebrtc_shim.so

# Default command: run tests
CMD ["go", "test", "-v", "./..."]
