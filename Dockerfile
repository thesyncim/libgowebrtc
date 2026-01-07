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
    ninja-build \
    rsync \
    lsb-release \
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

# Generate build configuration
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        mkdir -p ${BUILD_DIR}/src/out/Release && \
        echo 'is_debug = false' > ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'is_component_build = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'rtc_include_tests = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'proprietary_codecs = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'rtc_use_h264 = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'ffmpeg_branding = "Chrome"' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'rtc_include_dav1d_in_internal_decoder_factory = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'use_rtti = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'use_custom_libcxx = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'treat_warnings_as_errors = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'target_os = "linux"' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'target_cpu = "x64"' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'is_clang = true' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        echo 'use_sysroot = false' >> ${BUILD_DIR}/src/out/Release/args.gn && \
        cd ${BUILD_DIR}/src && gn gen out/Release; \
    fi

# Build libwebrtc (this is the slowest part)
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        cd ${BUILD_DIR}/src && ninja -C out/Release -j$(nproc) \
            :webrtc \
            api/video_codecs:builtin_video_decoder_factory \
            api/video_codecs:builtin_video_encoder_factory \
            api/audio_codecs:builtin_audio_decoder_factory \
            api/audio_codecs:builtin_audio_encoder_factory; \
    fi

# Install libwebrtc
RUN if [ "$SKIP_LIBWEBRTC_BUILD" != "1" ]; then \
        mkdir -p ${INSTALL_DIR}/{include,lib} && \
        cp ${BUILD_DIR}/src/out/Release/obj/libwebrtc.a ${INSTALL_DIR}/lib/; \
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
    libasound2 \
    libpulse0 \
    libx11-6 \
    libxext6 \
    libxrender1 \
    libdrm2 \
    libgbm1 \
    libegl1 \
    libgl1 \
    && rm -rf /var/lib/apt/lists/*

# Install Go
ARG GO_VERSION=1.25.3
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" | tar -C /usr/local -xzf -
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

# Build shim (only if libwebrtc is present)
RUN if [ -f /opt/libwebrtc/lib/libwebrtc.a ]; then \
        mkdir -p shim/build && \
        cd shim/build && \
        cmake .. \
            -DCMAKE_BUILD_TYPE=Release \
            -DLIBWEBRTC_DIR=${LIBWEBRTC_DIR} \
            -DBUILD_SHARED_LIBS=ON && \
        cmake --build . --config Release -j$(nproc) && \
        mkdir -p /workspace/lib/linux_amd64 && \
        cp libwebrtc_shim.so /workspace/lib/linux_amd64/; \
    else \
        echo "WARNING: libwebrtc not found, shim will not be built"; \
        echo "Mount your pre-built libwebrtc at /opt/libwebrtc"; \
    fi

# Set library path
ENV LIBWEBRTC_SHIM_PATH=/workspace/lib/linux_amd64/libwebrtc_shim.so

# Default command: run tests
CMD ["go", "test", "-v", "./..."]
