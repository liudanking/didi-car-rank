SERVICE := didi-car-rank
VERSION := 0.0.1



build:
	@echo "building..." && \
	go install && \
	echo "build done"


run-collect_data: build
	$(SERVICE) collect_data -d data






