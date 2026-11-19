// libwebp wrapper.
export module enginec.third_party.webp;

import std;

export namespace enginec::third_party::webp {

struct image {
    int width = 0;
    int height = 0;
    std::vector<std::uint8_t> bgra;
};

std::optional<image> decode(std::span<const std::uint8_t> data, std::string& error);

} // namespace enginec::third_party::webp
