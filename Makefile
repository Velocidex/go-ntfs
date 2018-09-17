all:
	go build -o ntfs bin/*.go


windows:
	GOOS=windows GOARCH=amd64 \
            go build \
	    -o ntfs.exe ./bin/*.go
