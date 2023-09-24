IMAGE_BUILD_CMD=$(shell which podman 2>/dev/null || which docker)

container-build:
	$(IMAGE_BUILD_CMD) build -t interop-ocp-watcher-bot .

container-run:
	$(IMAGE_BUILD_CMD) run -it --entrypoint /bin/bash interop-ocp-watcher-bot

container-build-run: container-build container-run
