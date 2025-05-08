FROM public.ecr.aws/amazonlinux/amazonlinux:latest as build-server

# Install dependencies
RUN yum install -y gcc-c++ gdb cmake3 git wget tar perl sudo

# Install correct OpenSSL version. NOTE: You might need to change this based on your Unreal Engine 5 version and the OpenSSL version it utilizes
RUN wget https://github.com/openssl/openssl/archive/refs/tags/OpenSSL_1_1_1n.tar.gz && \
tar -xzvf OpenSSL_1_1_1n.tar.gz && \
cd openssl-OpenSSL_1_1_1n/ && \
./config && \
sudo make install

RUN export LD_LIBRARY_PATH=/usr/local/lib:/usr/local/lib64 && \
openssl version && \
cd ..

# Copy the Open SSL files to binaries folder
RUN mkdir /binaries && \
cp ./openssl-OpenSSL_1_1_1n/libssl.so.1.1 /binaries/ && \
cp ./openssl-OpenSSL_1_1_1n/libcrypto.so.1.1 /binaries/ 

# Download and build the GameLift Server SDK (NOTE: You might need to change this to download a different SDK version)
RUN echo "Download and unzip GameLift Server SDK 5.2.0" && \
mkdir SDK && cd SDK && \
wget https://gamelift-server-sdk-release.s3.us-west-2.amazonaws.com/cpp/GameLift-Cpp-ServerSDK-5.2.0.zip && \
unzip GameLift-Cpp-ServerSDK-5.2.0.zip && \
echo "Build the GameLift server SDK" && \
mkdir cmake-build && \
cmake -G "Unix Makefiles" -DBUILD_FOR_UNREAL=1 -DCMAKE_BUILD_TYPE=Release -S . -B ./cmake-build && \
cmake --build cmake-build --target all && \
cd ./cmake-build/prefix/ && \
echo "Copying files over to binaries folder" && \
cp -r ./lib/*  /binaries/ && \
echo "copying over headers (these are already provided by the GameLift Unreal Plugin so you don't need them with that)" && \
mkdir /binaries/include && \
cp -r ./include/*  /binaries/include/

# Copy the binaries only to a clean setup for copying to local system after build
FROM scratch AS server
  COPY --from=build-server /binaries/ /
  ENTRYPOINT [ "/GameLiftSampleServer" ]

