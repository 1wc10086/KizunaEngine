// libpng wrapper. Only the 8-bit RGBA/BGRA paths the engine needs are exposed.
export module enginec.third_party.png;

import std;

export namespace enginec::third_party::png {

struct image {
    int width = 0;
    int height = 0;
    // Straight-alpha BGRA, matching the engine's bitmap layout.
    std::vector<std::uint8_t> bgra;
};

std::optional<image> decode(std::span<const std::uint8_t> data, std::string& error);
std::vector<std::uint8_t> encode(const image& source, std::string& error);

} // namespace enginec::third_party::png
