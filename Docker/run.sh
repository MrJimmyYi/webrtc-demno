docker network create \                                                                                                                                     ─╯
  --driver bridge \
  --subnet 172.18.0.0/16 \
  --gateway 172.18.0.1 \
  customize_network

docker run -d \                                                                                                                                             ─╯
  --name coturn \
  --network customize_network \
  --ip 172.18.0.10 \
  -p 23478:23478 \
  -p 23478:23478/udp \
  -p 49160-49200:49160-49200/udp \
  coturn:ubuntu20.04