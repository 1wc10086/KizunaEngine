// Error transport between modules.
//
// Exceptions never cross a module boundary in this project: the FFI catches them at
// the C ABI edge, but internal layers use `result<T>` so a failure carries a
// human-readable message and the caller decides whether it is fatal. This matches
// the Go engine's `(value, error)` shape and keeps the wire error text meaningful.
export module enginec.utility.error;

import std;

export namespace enginec {

struct error {
    std::string message;

    [[nodiscard]] bool empty() const noexcept { return message.empty(); }
    explicit operator bool() const noexcept { return !message.empty(); }
};

template <typename T>
class result {
public:
    result(T value) : value_(std::move(value)), ok_(true) {}
    result(error failure) : failure_(std::move(failure)), ok_(false) {}

    [[nodiscard]] bool has_value() const noexcept { return ok_; }
    explicit operator bool() const noexcept { return ok_; }
    T& value() & { return value_; }
    const T& value() const& { return value_; }
    T&& value() && { return std::move(value_); }
    [[nodiscard]] const error& failure() const noexcept { return failure_; }
    [[nodiscard]] std::string message() const { return failure_.message; }

private:
    T value_{};
    error failure_{};
    bool ok_ = false;
};

template <>
class result<void> {
public:
    result() : ok_(true) {}
    result(error failure) : failure_(std::move(failure)), ok_(false) {}

    [[nodiscard]] bool has_value() const noexcept { return ok_; }
    explicit operator bool() const noexcept { return ok_; }
    [[nodiscard]] const error& failure() const noexcept { return failure_; }
    [[nodiscard]] std::string message() const { return failure_.message; }

private:
    error failure_{};
    bool ok_ = false;
};

[[nodiscard]] inline error make_error(std::string message) { return error{std::move(message)}; }

} // namespace enginec
