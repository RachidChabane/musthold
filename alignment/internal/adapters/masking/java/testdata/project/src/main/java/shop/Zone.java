package shop;

public enum Zone {
    LOCAL, REMOTE;

    public boolean isRemote() {
        return this == REMOTE;
    }
}
