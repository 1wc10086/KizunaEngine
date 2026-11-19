// Host smoke tests for the utility and wire modules.
//
// These run on the build machine with no game data, so every input is synthetic. The
// checks are written with CHECK rather than assert(): the host preset builds in
// Release, where NDEBUG turns assert() into a no-op, and a test suite that silently
// asserts nothing is worse than no suite. A failing check prints file:line and a
// message and makes main() return 1, which is what ctest reports.

import std;

import enginec;

namespace {

int failures = 0;

void check(bool condition, const char* expression, const char* file, int line)
{
    if (condition)
    {
        return;
    }
    ++failures;
    std::cerr << file << ':' << line << ": check failed: " << expression << '\n';
}

} // namespace

#define CHECK(expr) check(static_cast<bool>(expr), #expr, __FILE__, __LINE__)

namespace {

std::uint16_t load_u16(std::span<const std::uint8_t> data, std::size_t offset)
{
    return static_cast<std::uint16_t>(
        static_cast<std::uint16_t>(data[offset]) |
        static_cast<std::uint16_t>(data[offset + 1] << 8));
}

void store_u16(std::vector<std::uint8_t>& out, std::size_t offset, std::uint16_t value)
{
    out[offset] = static_cast<std::uint8_t>(value);
    out[offset + 1] = static_cast<std::uint8_t>(value >> 8);
}

void store_u32(std::vector<std::uint8_t>& out, std::size_t offset, std::uint32_t value)
{
    for (int index = 0; index < 4; ++index)
    {
        out[offset + static_cast<std::size_t>(index)] =
            static_cast<std::uint8_t>(value >> (index * 8));
    }
}

std::vector<std::uint8_t> make_request(std::uint16_t op, std::uint32_t seq,
                                       const std::vector<std::uint8_t>& payload)
{
    std::vector<std::uint8_t> out(16, 0);
    out[0] = 'K';
    out[1] = 'Z';
    out[2] = 'E';
    out[3] = 'Q';
    store_u16(out, 4, op);
    store_u32(out, 8, seq);
    store_u32(out, 12, static_cast<std::uint32_t>(payload.size()));
    out.insert(out.end(), payload.begin(), payload.end());
    return out;
}

void test_wire_round_trip()
{
    using namespace enginec::wire;

    writer w;
    w.u32(field::f1, 0xDEADBEEFu);
    w.i32(field::f2, -7);
    w.u64(field::f3, 0x0123456789ABCDEFull);
    w.f64(field::f4, 2.5);
    w.boolean(field::f5, true);
    w.str(field::f7, "スクリプト");
    w.raw(field::f8, std::vector<std::uint8_t>{1, 2, 3});
    w.str_list(field::f9, {"a", "b"});
    // Both nested records live inside one list field: a second `list` call with the
    // same id would overwrite the first, because the reader keeps only the last value
    // of a duplicate id.
    w.list(field::f10, [](writer& sub) {
        sub.list(field::f1, [](writer& entry) {
            entry.u32(field::f1, 11);
            entry.u32(field::f2, 22);
        });
        sub.list(field::f1, [](writer& entry) { entry.u32(field::f1, 33); });
    });

    const reader r(w.data());
    CHECK(r.u32(field::f1) == 0xDEADBEEFu);
    CHECK(r.i32(field::f2) == -7);
    CHECK(r.u64(field::f3) == 0x0123456789ABCDEFull);
    CHECK(r.f64(field::f4) == 2.5);
    CHECK(r.boolean(field::f5));
    CHECK(r.str(field::f7) == "スクリプト");
    CHECK(r.raw(field::f8).size() == 3);
    CHECK(r.raw(field::f8).size() == 3 && r.raw(field::f8)[2] == 3);

    const auto list = r.str_list(field::f9);
    CHECK(list.size() == 2);
    CHECK(list.size() == 2 && list[0] == "a" && list[1] == "b");

    std::vector<std::pair<std::uint32_t, std::uint32_t>> entries;
    r.each(field::f10, [&entries](reader nested) {
        entries.emplace_back(nested.u32(field::f1), nested.u32(field::f2));
    });
    CHECK(entries.size() == 2);
    if (entries.size() == 2)
    {
        CHECK(entries[0].first == 11u && entries[0].second == 22u);
        CHECK(entries[1].first == 33u && entries[1].second == 0u);
    }

    // Absent and wrong-width fields read as zero/empty instead of touching memory.
    CHECK(!r.has(field::f11));
    CHECK(r.u32(field::f11) == 0);
    // f5 holds one byte, so a 64-bit read has no complete word pair to return.
    CHECK(r.u64(field::f5) == 0);
    CHECK(r.str(field::f11).empty());
    CHECK(!r.boolean(field::f11));

    // Reserved header bytes and field type ids are part of the wire contract; the Go
    // client writes 0 there and must stay readable.
    CHECK(w.data().size() > 8);
    CHECK(w.data()[3] == 0);
}

void test_wire_duplicate_id_last_wins()
{
    using namespace enginec::wire;

    writer w;
    w.u32(field::f1, 1);
    w.str(field::f12, "ignored");
    w.u32(field::f1, 2);

    const reader r(w.data());
    // The Go reader stores fields in a map, so the last occurrence is the visible one.
    CHECK(r.u32(field::f1) == 2);

    // A field whose declared length runs past the buffer ends the walk and must not be
    // admitted: this is the hostile-input case.
    std::vector<std::uint8_t> truncated(8, 0);
    store_u16(truncated, 0, field::f1);
    truncated[2] = static_cast<std::uint8_t>(type::str);
    store_u32(truncated, 4, 100);
    const reader hostile(truncated);
    CHECK(!hostile.has(field::f1));
}

void test_wire_request()
{
    using namespace enginec::wire;

    writer body;
    body.str(field::f7, "data.xp3");
    body.u32(field::f13, 4);
    const auto buffer = make_request(op_open, 42, body.data());

    const auto parsed = parse_request(buffer);
    CHECK(parsed.has_value());
    if (parsed)
    {
        CHECK(parsed->op == op_open);
        CHECK(parsed->seq == 42);
        CHECK(parsed->payload.str(field::f7) == "data.xp3");
        CHECK(parsed->payload.u32(field::f13) == 4);
    }

    // Short header, bad magic and a payload longer than the data are all rejected.
    CHECK(!parse_request(std::span<const std::uint8_t>(buffer).first(15)).has_value());
    auto bad_magic = buffer;
    bad_magic[0] = 'X';
    CHECK(!parse_request(bad_magic).has_value());
    auto short_payload = buffer;
    store_u32(short_payload, 12, static_cast<std::uint32_t>(short_payload.size()));
    CHECK(!parse_request(short_payload).has_value());

    const auto response = build_response(status_ok, 42, body.data());
    CHECK(response.size() == 16 + body.data().size());
    CHECK(response[0] == 'K' && response[1] == 'Z' && response[2] == 'E' && response[3] == 'S');
    CHECK(load_u16(response, 4) == status_ok);
    CHECK(load_u16(response, 6) == 0);

    const auto failure = build_error(7, "boom", status_not_found);
    CHECK(failure.size() > 16);
    CHECK(load_u16(failure, 4) == status_not_found);
    const auto failure_payload = parse_request(failure);
    // parse_request only accepts request magic, so decode the response body by hand.
    CHECK(!failure_payload.has_value());
    const reader error_body(std::span<const std::uint8_t>(failure).subspan(16));
    CHECK(error_body.str(field::f42) == "boom");
}

void test_utf16()
{
    // ASCII, Japanese and an astral character: the last one needs a surrogate pair, so
    // a naive unit-by-unit encoder would produce two lone surrogates instead.
    const std::string text = "hello 世界 😀 end";
    const auto units = enginec::utf8_to_utf16le(text);
    CHECK(units.size() % 2 == 0);
    // 15 UTF-16 code units: 21 UTF-8 bytes is not the reference because the three
    // three-byte characters and the four-byte emoji collapse.
    CHECK(units.size() == 15 * 2);

    // U+1F600 becomes D83D DE00 in little-endian order.
    bool found_pair = false;
    for (std::size_t index = 0; index + 3 < units.size(); index += 2)
    {
        if (units[index] == 0x3D && units[index + 1] == 0xD8 && units[index + 2] == 0x00 &&
            units[index + 3] == 0xDE)
        {
            found_pair = true;
        }
    }
    CHECK(found_pair);

    const auto round_trip = enginec::utf16le_to_utf8(units);
    CHECK(round_trip == text);

    // Unpaired surrogates and odd tails become U+FFFD (EF BF BD), never out-of-range
    // UTF-8.
    const std::vector<std::uint8_t> lone_high = {0x00, 0xD8};
    CHECK(enginec::utf16le_to_utf8(lone_high) == "\xEF\xBF\xBD");
    const std::vector<std::uint8_t> lone_low = {0x00, 0xDC};
    CHECK(enginec::utf16le_to_utf8(lone_low) == "\xEF\xBF\xBD");
    const std::vector<std::uint8_t> odd = {'A', 0x00, 'B'};
    CHECK(enginec::utf16le_to_utf8(odd) == "A\xEF\xBF\xBD");
    CHECK(enginec::utf16le_to_utf8(std::span<const std::uint8_t>{}).empty());

    // A truncated UTF-8 sequence encodes as one replacement character per bad byte,
    // matching Go's []rune conversion.
    const auto broken = enginec::utf8_to_utf16le(std::string_view("a\xFF" "b", 3));
    CHECK(enginec::utf16le_to_utf8(broken) == "a\xEF\xBF\xBD" "b");
}

void test_cp932()
{
    // "こんにちは、世界！テストｱｲｳ" as CP932. The halfwidth katakana at the end are
    // single bytes, so a decoder that only handles pairs would corrupt them.
    const std::string japanese = "こんにちは、世界！テストｱｲｳ";
    const std::vector<std::uint8_t> expected = {
        0x82, 0xB1, 0x82, 0xF1, 0x82, 0xC9, 0x82, 0xBF, 0x82, 0xCD, 0x81, 0x41,
        0x90, 0xA2, 0x8A, 0x45, 0x81, 0x49, 0x83, 0x65, 0x83, 0x58, 0x83, 0x67,
        0xB1, 0xB2, 0xB3,
    };
    const auto encoded = enginec::utf8_to_cp932(japanese);
    CHECK(encoded.size() == expected.size());
    CHECK(std::equal(encoded.begin(), encoded.end(), expected.begin(), expected.end()));

    CHECK(enginec::cp932_to_utf8(std::string(reinterpret_cast<const char*>(expected.data()),
                                             expected.size())) == japanese);

    // ASCII is identity in both directions.
    CHECK(enginec::cp932_to_utf8("data.xp3") == "data.xp3");
    CHECK(enginec::utf8_to_cp932("data.xp3") == "data.xp3");
    CHECK(enginec::cp932_to_utf8("").empty());
    CHECK(enginec::utf8_to_cp932("").empty());

    // A lead byte at the end of the buffer, and a lead followed by an invalid trail,
    // both decode to U+FFFD rather than reading past the end.
    CHECK(enginec::cp932_to_utf8(std::string_view("\x82", 1)) == "\xEF\xBF\xBD");
    CHECK(enginec::cp932_to_utf8(std::string_view("\x82\x0A", 2)) == "\xEF\xBF\xBD\n");

    // A code point outside CP932 (Hangul 가) degrades to '?', which keeps a save file
    // writable. Cyrillic Ж is representable (0x8447), so it must round-trip.
    CHECK(enginec::utf8_to_cp932("가") == "?");
    CHECK(enginec::cp932_to_utf8(enginec::utf8_to_cp932("Ж")) == "Ж");
}

void test_text_helpers()
{
    CHECK(enginec::trim("  \t\r\n abc \n") == "abc");
    CHECK(enginec::trim("\xE3\x80\x80" "x\xE3\x80\x80") == "x");
    CHECK(enginec::trim("   ").empty());
    CHECK(enginec::trim("").empty());

    CHECK(enginec::ascii_lower("DaTa.XP3") == "data.xp3");
    CHECK(enginec::ascii_lower("スクリプト") == "スクリプト");

    CHECK(enginec::equals_ignore_case("Data.xp3", "data.XP3"));
    CHECK(!enginec::equals_ignore_case("data.xp3", "data.xp4"));
    CHECK(!enginec::equals_ignore_case("a", "ab"));

    CHECK(enginec::normalize_storage_name("Data\\Script\\Main.KS") == "data/script/main.ks");
    CHECK(enginec::normalize_storage_name("data/script/main.ks") == "data/script/main.ks");
}

void test_md5()
{
    // RFC 1321 test suite, plus the 80-digit case that exercises multi-block input.
    const auto unknown = enginec::md5(std::span<const std::uint8_t>{});
    CHECK(unknown.size() == 16);
    CHECK(enginec::md5_hex("") == "d41d8cd98f00b204e9800998ecf8427e");
    CHECK(enginec::md5_hex("a") == "0cc175b9c0f1b6a831c399e269772661");
    CHECK(enginec::md5_hex("abc") == "900150983cd24fb0d6963f7d28e17f72");
    CHECK(enginec::md5_hex("message digest") == "f96b697d7cb7938d525a2f31aaf161d0");
    CHECK(enginec::md5_hex("abcdefghijklmnopqrstuvwxyz") == "c3fcd3d76192e4007dfb496cca67e13b");
    CHECK(enginec::md5_hex("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789") ==
          "d174ab98d277d9f5a5611c2c9f419d9f");
    CHECK(enginec::md5_hex("123456789012345678901234567890123456789012345678901234567890123456"
                           "78901234567890") == "57edf4a22be3c955ac49da2e2107b67a");

    // Padding boundaries: 55 leaves room for the length field in the same block, 56
    // and 57 force a second block, and 64/65 cover a full block plus an empty or
    // one-byte tail.
    CHECK(enginec::md5_hex(std::string(55, 'x')) == "04364420e25c512fd958a70738aa8f72");
    CHECK(enginec::md5_hex(std::string(56, 'x')) == "668a72d5ba17f08e62dabcafad6db14b");
    CHECK(enginec::md5_hex(std::string(57, 'x')) == "693037871c4a9d3d8685018905cb530a");
    CHECK(enginec::md5_hex(std::string(64, 'x')) == "c1bb4f81d892b2d57947682aeb252456");
    CHECK(enginec::md5_hex(std::string(65, 'x')) == "1bc932052302d074bdec39795fe00cf6");

    const auto empty = enginec::md5(std::span<const std::uint8_t>{});
    CHECK(empty[0] == 0xd4 && empty[1] == 0x1d && empty[15] == 0x7e);
    const auto abc = enginec::md5(std::span<const std::uint8_t>(
        reinterpret_cast<const std::uint8_t*>("abc"), 3));
    const std::array<std::uint8_t, 16> expected = {0x90, 0x01, 0x50, 0x98, 0x3c, 0xd2, 0x4f, 0xb0,
                                                   0xd6, 0x96, 0x3f, 0x7d, 0x28, 0xe1, 0x7f, 0x72};
    CHECK(abc == expected);

    // A digest longer than two blocks catches a block-count or length-field bug that
    // single-block vectors hide.
    CHECK(enginec::md5_hex("The quick brown fox jumps over the lazy dog. "
                           "The quick brown fox jumps over the lazy dog. "
                           "The quick brown fox jumps over the lazy dog. ") ==
          "e6f263d514adf884ef6cc864d206d462");
}

void test_result()
{
    enginec::result<int> good(7);
    CHECK(good.has_value());
    CHECK(static_cast<bool>(good));
    CHECK(good.value() == 7);
    CHECK(good.failure().empty());

    enginec::result<int> bad(enginec::make_error("nope"));
    CHECK(!bad.has_value());
    CHECK(!static_cast<bool>(bad));
    CHECK(bad.message() == "nope");

    enginec::result<void> done;
    CHECK(done.has_value());
    CHECK(static_cast<bool>(done));

    enginec::result<void> failed(enginec::make_error("open failed"));
    CHECK(!failed.has_value());
    CHECK(failed.message() == "open failed");
    CHECK(!failed.failure().empty());
}

void test_paths()
{
    CHECK(enginec::join_path("a", "b") == "a/b");
    CHECK(enginec::join_path("a/", "b") == "a/b");
    CHECK(enginec::join_path("", "b") == "b");
    CHECK(enginec::join_path("a", "") == "a");
    CHECK(enginec::join_path("a", "/abs") == "/abs");

    CHECK(enginec::parent_path("a/b/c.txt") == "a/b");
    CHECK(enginec::parent_path("c.txt").empty());
    CHECK(enginec::parent_path("").empty());

    CHECK(enginec::filename("a/b/c.txt") == "c.txt");
    CHECK(enginec::filename("a/b/") == "b");
    CHECK(enginec::filename("c.txt") == "c.txt");
    CHECK(enginec::filename("").empty());

    const auto stamp = enginec::current_time_iso8601();
    CHECK(stamp.size() == 20);
    CHECK(stamp.size() == 20 && stamp[4] == '-' && stamp[7] == '-' && stamp[10] == 'T' &&
          stamp[13] == ':' && stamp[16] == ':' && stamp[19] == 'Z');
    for (const auto character : stamp)
    {
        CHECK(character == '-' || character == 'T' || character == ':' || character == 'Z' ||
              (character >= '0' && character <= '9'));
    }
}

void test_files()
{
    std::error_code ec;
    const auto root = std::filesystem::temp_directory_path(ec) / "enginec-core-tests";
    if (ec)
    {
        // Without a writable temp directory the filesystem checks cannot run, which is
        // a broken environment rather than a broken implementation.
        std::cerr << "skipping file tests: " << ec.message() << '\n';
        return;
    }
    std::filesystem::remove_all(root, ec);

    const auto nested = enginec::join_path(enginec::join_path(root.string(), "saves"), "slot1");
    CHECK(enginec::make_directories(nested));
    CHECK(enginec::directory_exists(nested));
    CHECK(!enginec::file_exists(nested));
    CHECK(!enginec::directory_exists(enginec::join_path(nested, "missing")));

    const auto path = enginec::join_path(nested, "data.bin");
    const std::vector<std::uint8_t> payload = {0x00, 0x01, 0xFE, 0xFF, 0x7F};
    CHECK(enginec::write_file(path, payload));
    CHECK(enginec::file_exists(path));

    const auto read = enginec::read_file(path);
    CHECK(read.has_value());
    CHECK(read && *read == payload);

    // An empty file is a valid asset (some games ship zero-byte placeholders).
    const auto empty_path = enginec::join_path(nested, "empty.dat");
    CHECK(enginec::write_file(empty_path, std::span<const std::uint8_t>{}));
    const auto empty = enginec::read_file(empty_path);
    CHECK(empty.has_value());
    CHECK(empty && empty->empty());

    const auto listing = enginec::list_directory(nested);
    CHECK(listing.size() == 2);
    CHECK(std::is_sorted(listing.begin(), listing.end()));
    CHECK(listing.size() == 2 && listing[0] == "data.bin" && listing[1] == "empty.dat");

    CHECK(!enginec::read_file(enginec::join_path(nested, "missing.bin")).has_value());
    CHECK(!enginec::file_exists(enginec::join_path(nested, "missing.bin")));
    CHECK(enginec::list_directory(enginec::join_path(nested, "missing")).empty());
    CHECK(!enginec::make_directories(""));

    // Writing to a fresh path must create the missing parent chain.
    const auto deep = enginec::join_path(enginec::join_path(nested, "a/b"), "c.txt");
    CHECK(enginec::write_file(deep, payload));
    CHECK(enginec::file_exists(deep));

    std::filesystem::remove_all(root, ec);
}

void test_log()
{
    std::error_code ec;
    const auto root = std::filesystem::temp_directory_path(ec);
    if (ec)
    {
        return;
    }
    const auto path = enginec::join_path(enginec::join_path(root.string(), "enginec-log-tests"),
                                         "engine.log");
    std::filesystem::remove_all(enginec::parent_path(path), ec);

    enginec::set_log_file(path);
    CHECK(enginec::log_path() == path);
    enginec::log_info("hello log");
    enginec::log_error("bad thing");

    const auto contents = enginec::read_file(path);
    CHECK(contents.has_value());
    if (contents)
    {
        const std::string text(contents->begin(), contents->end());
        // Both lines are present, each starts with a timestamp, and info/error are
        // distinguishable.
        CHECK(text.find("hello log") != std::string::npos);
        CHECK(text.find("bad thing") != std::string::npos);
        CHECK(text.find(" I hello log") != std::string::npos);
        CHECK(text.find(" E bad thing") != std::string::npos);
        CHECK(text.size() > 20);
    }

    // Appending, not truncating: a second run must not erase the first line.
    enginec::set_log_file(path);
    enginec::log_info("second run");
    const auto appended = enginec::read_file(path);
    CHECK(appended.has_value());
    if (appended)
    {
        const std::string text(appended->begin(), appended->end());
        CHECK(text.find("hello log") != std::string::npos);
        CHECK(text.find("second run") != std::string::npos);
    }

    enginec::set_log_file("");
    CHECK(enginec::log_path().empty());
    std::filesystem::remove_all(enginec::parent_path(path), ec);
}

} // namespace

int main()
{
    test_wire_round_trip();
    test_wire_duplicate_id_last_wins();
    test_wire_request();
    test_utf16();
    test_cp932();
    test_text_helpers();
    test_md5();
    test_result();
    test_paths();
    test_files();
    test_log();

    if (failures != 0)
    {
        std::cerr << failures << " check(s) failed\n";
        return 1;
    }
    std::cout << "enginec core tests passed\n";
    return 0;
}
