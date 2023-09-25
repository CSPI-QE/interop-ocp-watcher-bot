IMAGE_BUILD_CMD=$(shell which podman 2>/dev/null || which docker)

container-build:
	$(IMAGE_BUILD_CMD) build -t interop-ocp-watcher-bot .

container-terminal:
	$(IMAGE_BUILD_CMD) run -it --entrypoint /bin/bash interop-ocp-watcher-bot

container-execute:
	$(IMAGE_BUILD_CMD) run -it --entrypoint interop-ocp-watcher-bot interop-ocp-watcher-bot --job_file_path=$(job_file_path) --mentioned_group_id=$(mentioned_group_id) --webhook_url=$(webhook_url) --job_group_name=$(job_group_name)

container-build-terminal:
	container-build container-terminal

container-build-execute:
	container-build container-execute

pre-commit:
	pre-commit run --all-files
