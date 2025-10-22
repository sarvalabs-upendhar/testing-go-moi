[coco]
version = "0.7.0-rc.3"

[module]
name = "Answer"
version = "0.0.1"
license = []
repository = ""
authors = []

[target]
os = "MOI"
arch = "PISA"

[target.moi]
format = "YAML"
output = "answer"

[target.pisa]
format = "BIN"
version = "0.5.0"

[lab.render]
big_int_as_hex = true
bytes_as_hex = false

[lab.config.default]
url = "http://127.0.0.1:6060"
env = "main"

[lab.scripts]
test-toggle = ["engines", "users", "logics"]

[scripts]
test-script = "coco compile .; pwd; uname -a"
