![banner](.github/banner.png)

[moitech]: https://moi.technology/
[moidocs]: https://docs.moi.technology/

[![moi](https://img.shields.io/badge/moi-WEBSITE-purple?style=for-the-badge&color=%234B17E5)][moitech]
[![docs](https://img.shields.io/badge/moi-Documentation-purple?style=for-the-badge&color=%234B17E5)][moidocs]
![release](https://img.shields.io/badge/release-Babylon-blue?style=for-the-badge)
![codecov](https://img.shields.io/codecov/c/github/sarvalabs/moichain?token=7EKUYID0LM&style=for-the-badge)

# go-moi
Official Go Implementation of the MOI Protocol.

## Installing from the source
Installing `moipod` requires both a Go (version 1.18) and a C compiler. You can install
them using your favourite package manager. Once the dependencies are installed, run

```shell
make moipod
```

or, to install the full suite of utilities:

```shell
make install
```

## Executables
The go-moi project comes bundled with multiple executables and tools found in the `cmd` directory.

|   Command    | Description                                                                                                                                | Documentation                                                         |
|:------------:|--------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------|
| **`moipod`** | Our main MOI CLI client. It is the entry point into the MOI network, capable of running as a full Guardian Node or a Bootstrap Node.       | [MOIPod CLI Docs](https://docs.moi.technology/docs/guard/moipod-cli)  |
|  `mcutils`   | Utility tool for testing, generating genesis files and test directories.                                                                   | n/a                                                                   |
|  `logiclab`  | Sandbox playground environment for compiling manifest into logics, simulating logic calls and participant interactions with logics on MOI. | [LogicLab CLI Docs](https://docs.moi.technology/docs/build/logiclab)  |


## MOI Pod Docker Image
`moipod` can be installed and run by pulling the Docker image,
```shell
docker pull sarvalabs/moipod
```