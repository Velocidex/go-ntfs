all:
	go build -o ntfs bin/*.go


windows:
	GOOS=windows GOARCH=amd64 \
            go build \
	    -o ntfs.exe ./bin/*.go

generate:
	cd parser/ && binparsegen conversion.spec.yaml > ntfs_gen.go


test:
	go test ./...
