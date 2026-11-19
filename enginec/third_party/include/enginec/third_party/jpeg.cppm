// libjpeg-turbo wrapper. Decodes baseline/progressive JPEG into BGRA.
export module enginec.third_party.jpeg;

import std;

export namespace enginec::third_party::jpeg {

struct image {
    int width = 0;
    int height = 0;
    std::vector<std::uint8_t> bgra;
};

std::optional<image> decode(std::span<const std::uint8_t> data, std::string& error);

} // namespace enginec::third_party::jpeg
