// minizip-ng wrapper for ZIP containers (some KiriKiri patch archives are ZIP).
export module enginec.third_party.minizip;

import std;

export namespace enginec::third_party::minizip {

struct entry {
    std::string name;
    std::uint64_t size = 0;
    std::uint64_t compressed_size = 0;
};

class archive {
public:
    archive();
    ~archive();
    archive(const archive&) = delete;
    archive& operator=(const archive&) = delete;
    archive(archive&&) noexcept;
    archive& operator=(archive&&) noexcept;

    bool open(std::string_view path, std::string& error);
    void close();
    [[nodiscard]] const std::vector<entry>& entries() const noexcept;
    std::optional<std::vector<std::uint8_t>> read(std::string_view name, std::string& error) const;

private:
    struct impl;
    std::unique_ptr<impl> impl_;
};

} // namespace enginec::third_party::minizip
