name: build_docker

on:
#  push:
#    branches:
#      - 'main'
#    tags:
#      - '*'
 workflow_dispatch:

jobs:
  build-docker-image:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v2

      - name: Login to Docker Hub
        uses: docker/login-action@v1
        with:
          username: ${{ secrets.DOCKER_HUB_USERNAME }}
          password: ${{ secrets.DOCKER_HUB_ACCESS_TOKEN }}

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v2

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v2

      - name: Build and push
        uses: docker/build-push-action@v2
        with:
          context: .
          file: ./docker/Dockerfile
          builder: ${{ steps.buildx.outputs.name }}
          platforms: linux/arm64,linux/amd64
          push: true
          tags: ${{ secrets.DOCKER_HUB_USERNAME }}/focalboard:${{ github.ref_name }}
          cache-from: type=registry,ref=${{ secrets.DOCKER_HUB_USERNAME }}/af_build_cache:buildcache
          cache-to: type=registry,ref=${{ secrets.DOCKER_HUB_USERNAME }}/af_build_cache:buildcache,mode=max
