// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "TokenDogBar",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .executable(name: "TokenDogBar", targets: ["TokenDogBar"])
    ],
    targets: [
        // No external dependencies: the app uses only system frameworks
        // (AppKit + ServiceManagement) and shells out to the `td` CLI.
        .executableTarget(
            name: "TokenDogBar",
            path: "Sources/TokenDogBar"
        )
    ]
)
