#include <stdlib.h>
#include <unistd.h>
#include <string.h>
#include <time.h>

void allocateAndFreeAfter(size_t totalSize, int seconds) {
    void* block = malloc(totalSize);
    if (block == NULL) {
        write(1, "malloc failed\n", 14);
        // Handle allocation failure
        return;
    }

    // Seed the random number generator
    srand((unsigned)time(NULL));

    const int blockSize = 1024; // Small block size for initial random data
    unsigned char* smallBlock = malloc(blockSize);
    if (smallBlock == NULL) {
        // Handle allocation failure
        free(block);
        return;
    }

    // Generate random data for the small block
    for (int i = 0; i < blockSize; ++i) {
        smallBlock[i] = rand() % 256;
    }

    // Replicate the small block across the larger block
    for (int offset = 0; offset < totalSize; offset += blockSize) {
        int copySize = blockSize < totalSize - offset ? blockSize : totalSize - offset;
        memcpy((unsigned char*)block + offset, smallBlock, copySize);
    }

    // Free the small block after use
    free(smallBlock);

    // Simulate workload
    sleep(seconds);

    // Free the large block
    free(block);
}