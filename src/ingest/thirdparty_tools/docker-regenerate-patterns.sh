cd ..
docker run -it --name ingestgen --rm -v ./:/mnt/ ubuntu:22.04 bash -c "apt update && apt -y install python-is-python3 python3-yaml && cd /mnt/thirdparty_tools/ && python3 log_parse_regex_transformer.py all" 
